package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

var (
	version = "1.1.0"
)

func main() {
	var (
		portFlag int
		hostFlag string
		showVer  bool
	)

	flag.IntVar(&portFlag, "p", 0, "Web 服务监听端口 (默认 0 为随机生成 10000-60000)")
	flag.IntVar(&portFlag, "port", 0, "Web 服务监听端口 (默认 0 为随机生成 10000-60000)")
	flag.StringVar(&hostFlag, "h", "0.0.0.0", "Web 服务绑定主机地址")
	flag.StringVar(&hostFlag, "host", "0.0.0.0", "Web 服务绑定主机地址")
	flag.BoolVar(&showVer, "v", false, "显示版本号")
	flag.BoolVar(&showVer, "version", false, "显示版本号")
	flag.Parse()

	if showVer {
		fmt.Printf("nftables_control_panel v%s\n", version)
		os.Exit(0)
	}

	fmt.Println("==================================================")
	fmt.Printf("       nftables Control Panel v%s\n", version)
	fmt.Println("       开源地址: https://github.com/M0eBa1/nftables_control_panel")
	fmt.Println("==================================================")

	// 1. 检查并自动安装 nftables
	if err := CheckAndInstallNftables(); err != nil {
		log.Fatalf("❌ 环境检查错误: %v", err)
	}

	// 2. 检查并开启 IPv4 转发
	if err := CheckAndEnableIPv4Forward(); err != nil {
		log.Printf("⚠️ 开启 IPv4 转发警告: %v", err)
	}

	// 3. 确定监听端口 (如果不给运行参数，默认随机 10000-60000)
	finalPort := portFlag
	if finalPort <= 0 {
		finalPort = getRandomAvailablePort(hostFlag)
	}

	// 4. 自动放行控制面板自身端口与 SSH 22，防止防火墙把自己封死
	EnsureSafePorts(finalPort)

	bindAddr := fmt.Sprintf("%s:%d", hostFlag, finalPort)

	// 打印访问信息
	fmt.Println("\n🚀 控制面板已成功启动!")
	fmt.Printf("  ▶ 本机访问: http://127.0.0.1:%d\n", finalPort)
	localIPs := GetLocalIPs()
	for _, ip := range localIPs {
		fmt.Printf("  ▶ 网络访问: http://%s:%d\n", ip, finalPort)
	}
	fmt.Println("--------------------------------------------------")
	fmt.Println("  按 Ctrl+C 可停止服务")
	fmt.Println("==================================================\n")

	// 优雅终止监听
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		fmt.Println("\n[Server] 接收到退出信号，正在关闭服务...")
		os.Exit(0)
	}()

	serverCfg := ServerConfig{
		BindAddr: bindAddr,
		Port:     finalPort,
	}

	if err := StartHTTPServer(serverCfg); err != nil {
		log.Fatalf("❌ HTTP 服务启动失败: %v", err)
	}
}

// getRandomAvailablePort 获取一个 10000 到 60000 之间的随机可用端口
func getRandomAvailablePort(host string) int {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 50; i++ {
		candidate := r.Intn(50000) + 10000
		addr := net.JoinHostPort(host, strconv.Itoa(candidate))
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			listener.Close()
			return candidate
		}
	}
	// 兜底让系统分配
	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err == nil {
		defer listener.Close()
		return listener.Addr().(*net.TCPAddr).Port
	}
	return 28443
}
