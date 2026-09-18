// ccodex-sleep-state is a local, single-process Codex turn-state service.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gylive/ccodex-sleep-state/internal/codexconfig"
	"github.com/gylive/ccodex-sleep-state/internal/fsutil"
	"github.com/gylive/ccodex-sleep-state/internal/instance"
	"github.com/gylive/ccodex-sleep-state/internal/proxyroute"
	"github.com/gylive/ccodex-sleep-state/internal/service"
	"github.com/gylive/ccodex-sleep-state/internal/settings"
)

var version = "dev"

const help = `ccodex-sleep-state — 一个本地服务，只处理 Astra

  ccodex-sleep-state init     创建本程序配置，暂不修改 Codex。
  ccodex-sleep-state serve    启动服务，备份并接管 Codex 配置；Ctrl+C 恢复。
  ccodex-sleep-state status   从正在运行的服务读取状态。
  ccodex-sleep-state check    检查配置和订阅，不发送模型请求。
  ccodex-sleep-state restore  崩溃后恢复 Codex 配置，不覆盖用户的新修改。
  ccodex-sleep-state paths    显示本机的配置与日志目录。
  ccodex-sleep-state version  显示版本。

选项放在命令之后：
  --data-dir PATH             使用独立的服务数据目录。
  --config PATH               使用指定 JSON 配置；init 会创建这个文件。
  --no-config                仅限 serve：不接管 Codex 配置。

主动探测使用你已有的 Codex 登录，会消耗实际额度。探测次数受限，
遇到 401、403、429 停止本轮。日志不记录账号、订阅、正文和完整 token。
第一次部署前，请阅读 docs/windows.md 与 docs/proxies.md。
`

func main() {
	proxyroute.QuietCore()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "错误：", err)
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Fprint(out, help)
		return nil
	}
	if args[0] == "version" {
		fmt.Fprintln(out, version)
		return nil
	}
	defaultDir, err := settings.DataDir()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(out)
	dir := flags.String("data-dir", defaultDir, "service data directory")
	configPath := flags.String("config", "", "service config path")
	noConfig := flags.Bool("no-config", false, "do not edit Codex config")
	if err = flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *noConfig && args[0] != "serve" {
		return errors.New("--no-config is only valid with serve")
	}
	*dir, err = filepath.Abs(*dir)
	if err != nil {
		return err
	}
	if *configPath == "" {
		*configPath = filepath.Join(*dir, "config.json")
	}
	switch args[0] {
	case "paths":
		c := settings.Default()
		if _, err := os.Stat(*configPath); err == nil {
			c, err = settings.Load(*configPath)
			if err != nil {
				return err
			}
		}
		home, err := c.CodexDir()
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "服务数据：%s\n服务配置：%s\nCodex 配置：%s\n日志目录：%s\n", *dir, *configPath, filepath.Join(home, "config.toml"), filepath.Join(*dir, "logs"))
		return nil
	case "init":
		if _, err = os.Lstat(*configPath); err == nil {
			return errors.New("config already exists; it was not overwritten")
		} else if !os.IsNotExist(err) {
			return err
		}
		data, _ := json.MarshalIndent(settings.Default(), "", "  ")
		if err = fsutil.Create(*configPath, append(data, '\n')); err != nil {
			return err
		}
		fmt.Fprintln(out, "已创建", *configPath)
		return nil
	case "status":
		data, err := service.Status(ctx, *dir)
		if err != nil {
			return err
		}
		_, err = out.Write(data)
		return err
	case "restore":
		lock, err := instance.Acquire(*dir)
		if err != nil {
			return err
		}
		defer lock.Close()
		if err = codexconfig.Restore(*dir); err != nil {
			return err
		}
		fmt.Fprintln(out, "配置已恢复，或当前没有需要恢复的记录。")
		return nil
	case "serve", "check":
		c, err := settings.Load(*configPath)
		if err != nil {
			return err
		}
		if args[0] == "serve" {
			return service.Run(ctx, *dir, c, !*noConfig, out)
		}
		routes, err := proxyroute.Load(ctx, c)
		if err != nil {
			return err
		}
		for _, r := range routes {
			r.Close()
		}
		fmt.Fprintf(out, "配置有效：%d 个出站配置。没有发送模型请求。\n", len(routes))
		return nil
	default:
		return errors.New("unknown command; run help")
	}
}
