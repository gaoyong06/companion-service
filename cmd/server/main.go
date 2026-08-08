package main

import (
	"flag"
	"fmt"
	"os"

	"companion-service/internal/conf"

	"github.com/gaoyong06/go-pkg/logger"
	pkgutils "github.com/gaoyong06/go-pkg/utils"
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	kratosgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	_ "go.uber.org/automaxprocs"
)

var (
	flagconf string
	runMode  string
	id, _    = os.Hostname()
	Name     = "companion-service"
	Version  string
)

func init() {
	flag.StringVar(&flagconf, "conf", "", "config path, eg: -conf config.yaml")
	flag.StringVar(&runMode, "mode", "debug", "Run mode (debug, release)")
}

func newApp(logger log.Logger, hs *kratoshttp.Server, gs *kratosgrpc.Server) *kratos.App {
	return kratos.New(kratos.ID(id), kratos.Name(Name), kratos.Version(Version), kratos.Logger(logger), kratos.Server(hs, gs))
}

func main() {
	flag.Parse()
	configPath := flagconf
	if configPath == "" {
		configPath = pkgutils.FindConfigFileWithMode(runMode, []string{"configs", "../../configs", "../configs"})
	}
	c := config.New(config.WithSource(file.NewSource(configPath)))
	defer c.Close()
	if err := c.Load(); err != nil {
		panic(fmt.Sprintf("load config failed: %v", err))
	}
	var bootstrap conf.Bootstrap
	if err := c.Scan(&bootstrap); err != nil {
		panic(fmt.Sprintf("scan config failed: %v", err))
	}
	logConfig := &logger.Config{Level: bootstrap.Log.Level, Format: bootstrap.Log.Format, Output: bootstrap.Log.Output, FilePath: bootstrap.Log.FilePath, MaxSize: int(bootstrap.Log.MaxSize), MaxAge: int(bootstrap.Log.MaxAge), MaxBackups: int(bootstrap.Log.MaxBackups), Compress: bootstrap.Log.Compress}
	loggerInstance, _ := logger.InitLogger(logConfig, id, Name, Version)
	app, cleanup, err := wireApp(&bootstrap, loggerInstance)
	if err != nil {
		panic(fmt.Sprintf("wire application failed: %v", err))
	}
	defer cleanup()
	if err := app.Run(); err != nil {
		panic(fmt.Sprintf("run application failed: %v", err))
	}
}
