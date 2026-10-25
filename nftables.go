package main

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ForwardRule 代表一条端口转发规则
type ForwardRule struct {
	Handle   int    `json:"handle"`
	Protocol string `json:"protocol"` // tcp, udp
	SrcPort  int    `json:"src_port"`
	DstHost  string `json:"dst_host"` // 空代表本机重定向
	DstPort  int    `json:"dst_port"`
	Type     string `json:"type"`    // dnat / redirect
	Comment  string `json:"comment"`
	Raw      string `json:"raw"`
}

// FilterRule 代表一条过滤规则
type FilterRule struct {
	Handle   int    `json:"handle"`
	Chain    string `json:"chain"` // input, forward, output
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	Action   string `json:"action"` // accept, drop
	Comment  string `json:"comment"`
	Raw      string `json:"raw"`
}

// CheckAndInstallNftables 检查并在缺少时自动安装 nftables
func CheckAndInstallNftables() error {
	_, err := exec.LookPath("nft")
	if err == nil {
		return nil
	}

	log.Println("[Init] 未找到 nft 命令，正在检测系统包管理器并自动安装 nftables...")

	managers := []struct {
		name string
		cmd  string
		args []string
	}{
		{"apk", "apk", []string{"add", "--no-cache", "nftables"}},
		{"apt-get", "apt-get", []string{"update", "-y"}}, // followed by apt-get install -y nftables
		{"dnf", "dnf", []string{"install", "-y", "nftables"}},
		{"yum", "yum", []string{"install", "-y", "nftables"}},
		{"pacman", "pacman", []string{"-Sy", "--noconfirm", "nftables"}},
		{"zypper", "zypper", []string{"install", "-y", "nftables"}},
	}

	installed := false
	for _, m := range managers {
		if _, err := exec.LookPath(m.cmd); err == nil {
			log.Printf("[Init] 使用 %s 尝试安装 nftables...", m.name)
			if m.name == "apt-get" {
				exec.Command("apt-get", "update", "-y").Run()
				cmd := exec.Command("apt-get", "install", "-y", "nftables")
				cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
				if out, err := cmd.CombinedOutput(); err == nil {
					installed = true
					break
				} else {
					log.Printf("[Init] %s 安装失败: %s, 输出: %s", m.name, err, string(out))
				}
			} else {
				cmd := exec.Command(m.cmd, m.args...)
				if out, err := cmd.CombinedOutput(); err == nil {
					installed = true
					break
				} else {
					log.Printf("[Init] %s 安装失败: %s, 输出: %s", m.name, err, string(out))
				}
			}
		}
	}

	if !installed {
		return fmt.Errorf("未能通过已知包管理器安装 nftables，请手动执行安装")
	}

	// 启动并使能 nftables 服务
	if _, err := exec.LookPath("systemctl"); err == nil {
		exec.Command("systemctl", "enable", "--now", "nftables").Run()
	} else if _, err := exec.LookPath("rc-service"); err == nil {
		exec.Command("rc-service", "nftables", "start").Run()
		exec.Command("rc-update", "add", "nftables", "default").Run()
	}

	log.Println("[Init] nftables 安装并启动成功")
	return nil
}

// CheckAndEnableIPv4Forward 检查并自动开启 IPv4 转发
func CheckAndEnableIPv4Forward() error {
	data, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
	enabled := false
	if err == nil && strings.TrimSpace(string(data)) == "1" {
		enabled = true
	}

	if !enabled {
		log.Println("[Init] 检测到 net.ipv4.ip_forward 未开启，正在启用...")
		// 1. 运行时生效
		if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1\n"), 0644); err != nil {
			// 回退尝试 sysctl -w
			exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()
		}

		// 2. 持久化到 /etc/sysctl.d/99-nftables-forward.conf
		sysctlConfPath := "/etc/sysctl.d/99-nftables-forward.conf"
		_ = os.WriteFile(sysctlConfPath, []byte("net.ipv4.ip_forward=1\n"), 0644)

		// 3. 兼容 /etc/sysctl.conf
		if confBytes, err := os.ReadFile("/etc/sysctl.conf"); err == nil {
			if !strings.Contains(string(confBytes), "net.ipv4.ip_forward") {
				f, err := os.OpenFile("/etc/sysctl.conf", os.O_APPEND|os.O_WRONLY, 0644)
				if err == nil {
					_, _ = f.WriteString("\nnet.ipv4.ip_forward=1\n")
					f.Close()
				}
			}
		}

		// 执行 sysctl -p 刷新
		exec.Command("sysctl", "-p").Run()
		log.Println("[Init] net.ipv4.ip_forward 已经成功开启并持久化配置")
	} else {
		log.Println("[Init] net.ipv4.ip_forward 已处于开启状态")
	}
	return nil
}

// EnsureBaseTables 确保 NAT 表和必要的链结构存在
func EnsureBaseTables() error {
	// 创建 ip nat 表和基本链
	cmds := [][]string{
		{"add", "table", "ip", "nat"},
		{"add", "chain", "ip", "nat", "prerouting", "{ type nat hook prerouting priority -100; policy accept; }"},
		{"add", "chain", "ip", "nat", "postrouting", "{ type nat hook postrouting priority 100; policy accept; }"},
		// 确保 postrouting 中包含一条基本的 masquerade
		{"add", "rule", "ip", "nat", "postrouting", "masquerade"},
	}

	for _, c := range cmds {
		_ = runNft(c...)
	}

	return nil
}

// runNft 执行 nft 命令
func runNft(args ...string) error {
	cmd := exec.Command("nft", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nft %s 失败: %w, 错误输出: %s", strings.Join(args, " "), err, stderr.String())
	}
	return nil
}

// runNftOutput 执行 nft 并返回标准输出
func runNftOutput(args ...string) (string, error) {
	cmd := exec.Command("nft", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("nft %s 失败: %w, 详情: %s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}

// GetForwardRules 获取所有端口转发规则
func GetForwardRules() ([]ForwardRule, error) {
	out, err := runNftOutput("-a", "list", "chain", "ip", "nat", "prerouting")
	if err != nil {
		// 如果表不存在，尝试先初始化
		_ = EnsureBaseTables()
		out, err = runNftOutput("-a", "list", "chain", "ip", "nat", "prerouting")
		if err != nil {
			return nil, err
		}
	}

	var rules []ForwardRule
	lines := strings.Split(out, "\n")

	// 正则匹配示例:
	// tcp dport 8443 dnat to 192.168.1.10:443 comment "test" # handle 5
	// udp dport 8443 redirect to :8080 # handle 6
	reHandle := regexp.MustCompile(`# handle (\d+)`)
	reComment := regexp.MustCompile(`comment "(.*?)"`)
	reDnat := regexp.MustCompile(`(tcp|udp)\s+dport\s+(\d+)\s+dnat\s+to\s+([a-zA-Z0-9\.\_\-]+)(?::(\d+))?`)
	reRedirect := regexp.MustCompile(`(tcp|udp)\s+dport\s+(\d+)\s+redirect(?:\s+to\s+:?(\d+))?`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "dnat") && !strings.Contains(line, "redirect") {
			continue
		}

		rule := ForwardRule{Raw: line}

		// 解析 handle
		if m := reHandle.FindStringSubmatch(line); len(m) > 1 {
			rule.Handle, _ = strconv.Atoi(m[1])
		}

		// 解析 comment
		if m := reComment.FindStringSubmatch(line); len(m) > 1 {
			rule.Comment = m[1]
		}

		// 解析 dnat
		if m := reDnat.FindStringSubmatch(line); len(m) > 1 {
			rule.Protocol = m[1]
			rule.SrcPort, _ = strconv.Atoi(m[2])
			rule.DstHost = m[3]
			if len(m) > 4 && m[4] != "" {
				rule.DstPort, _ = strconv.Atoi(m[4])
			} else {
				rule.DstPort = rule.SrcPort
			}
			rule.Type = "dnat"
			rules = append(rules, rule)
			continue
		}

		// 解析 redirect
		if m := reRedirect.FindStringSubmatch(line); len(m) > 1 {
			rule.Protocol = m[1]
			rule.SrcPort, _ = strconv.Atoi(m[2])
			rule.DstHost = "127.0.0.1"
			if len(m) > 3 && m[3] != "" {
				rule.DstPort, _ = strconv.Atoi(m[3])
			} else {
				rule.DstPort = rule.SrcPort
			}
			rule.Type = "redirect"
			rules = append(rules, rule)
			continue
		}
	}

	return rules, nil
}

// AddForwardRule 添加一条或两条(tcp+udp)端口转发规则
func AddForwardRule(protocol string, srcPort int, dstHost string, dstPort int, comment string) error {
	if err := EnsureBaseTables(); err != nil {
		return err
	}

	protos := []string{}
	switch strings.ToLower(protocol) {
	case "tcp":
		protos = []string{"tcp"}
	case "udp":
		protos = []string{"udp"}
	case "both", "all", "tcp+udp":
		protos = []string{"tcp", "udp"}
	default:
		return fmt.Errorf("不支持的协议类型: %s", protocol)
	}

	if dstPort <= 0 {
		dstPort = srcPort
	}

	for _, p := range protos {
		args := []string{"add", "rule", "ip", "nat", "prerouting", p, "dport", strconv.Itoa(srcPort)}
		if dstHost == "" || dstHost == "127.0.0.1" || dstHost == "localhost" {
			args = append(args, "redirect", "to", fmt.Sprintf(":%d", dstPort))
		} else {
			args = append(args, "dnat", "to", fmt.Sprintf("%s:%d", dstHost, dstPort))
		}

		if comment != "" {
			args = append(args, "comment", comment)
		}

		if err := runNft(args...); err != nil {
			return err
		}
	}

	return SaveRulesetToFile()
}

// DeleteForwardRuleByHandle 删除指定 handle 的规则
func DeleteForwardRuleByHandle(handle int) error {
	if handle <= 0 {
		return fmt.Errorf("无效的 handle 编号")
	}
	err := runNft("delete", "rule", "ip", "nat", "prerouting", "handle", strconv.Itoa(handle))
	if err != nil {
		return err
	}
	return SaveRulesetToFile()
}

// GetRawRuleset 获取完整规则集
func GetRawRuleset() (string, error) {
	return runNftOutput("list", "ruleset")
}

// SaveRulesetToFile 保存当前内存规则集到 /etc/nftables.conf
func SaveRulesetToFile() error {
	out, err := GetRawRuleset()
	if err != nil {
		return err
	}

	confPath := "/etc/nftables.conf"
	// 备份现有配置
	if current, err := os.ReadFile(confPath); err == nil && len(current) > 0 {
		_ = os.WriteFile(confPath+".bak", current, 0644)
	}

	content := fmt.Sprintf("#!/usr/sbin/nft -f\n# Generated by nftables_control_panel at %s\nflush ruleset\n\n%s\n",
		time.Now().Format("2006-01-02 15:04:05"), strings.TrimSpace(out))

	return os.WriteFile(confPath, []byte(content), 0644)
}

// FlushAllRules 清空所有规则
func FlushAllRules() error {
	return runNft("flush", "ruleset")
}

// EnsureSafePorts 放行指定的本地端口（例如当前 Web 服务端口与 SSH 22）
func EnsureSafePorts(port int) {
	// 尝试向 inet filter input 添加 accept 规则防把自己锁了
	_ = runNft("add", "table", "inet", "filter")
	_ = runNft("add", "chain", "inet", "filter", "input", "{ type filter hook input priority 0; policy accept; }")
	_ = runNft("add", "rule", "inet", "filter", "input", "tcp", "dport", "22", "accept", "comment", "SSH-Safe")
	if port > 0 {
		_ = runNft("add", "rule", "inet", "filter", "input", "tcp", "dport", strconv.Itoa(port), "accept", "comment", "WebPanel-Safe")
	}
}

// GetLocalIPs 获取所有非回环的本地 IPv4 地址
func GetLocalIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ips = append(ips, ipnet.IP.String())
			}
		}
	}
	return ips
}
