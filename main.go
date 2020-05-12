package main

import (
	"net/http"
	_ "think/docs"
	"think/router"
	"think/scheduler"
	"think/setting"
)

// @title think API
// @version 1.0
// @description 加锁的API需要Authorization
// @host 127.0.0.1:15001
// @BasePath /api
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
func main() {
	//初始化API
	r := router.Init()
	//运行scheduler
	go scheduler.Run(setting.Config.TCPAddr)
	err := http.ListenAndServe(setting.Config.APIAddr, r)
	if err != nil {
		panic(err)
	}
}
