package scheduler

import (
	"encoding/hex"
	"fmt"
	"log"
	mqclient "think/mqtt/client"

	"strings"
	"think/models"
	"think/tool"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func consumer() {

	// 订阅MQTT
	f := func(client mqtt.Client, msg mqtt.Message) {
		topic := msg.Topic()
		idx := strings.Split(topic, "/")
		devID := idx[1]
		crc16 := tool.CRCString(msg.Payload())
		if crc16 == "0000" {
			return
		}
		if devID != "" {
			// 封装 DeviceTask
			task := make([]models.Task, 1)
			task[0] = models.Task{Query: fmt.Sprintf("%s%s", msg.Payload(), crc16)}

			devTask := &models.DeviceTask{DevID: devID, Tasks: task}
			go write(devTask)
		}
	}

	if token := mqclient.Client.Subscribe("dt/+/query", 0, f); token.Wait() && token.Error() != nil {
		fmt.Println(token.Error())
	}
	for {
		msg := <-QueryCH
		//启动协程发送消息

		go write(msg)
	}
}

func write(dt *models.DeviceTask) {
	// 判断DevConn是否存在
	devConn, ok := ID2DevConnMap[dt.DevID]
	if ok {
		for _, v := range dt.Tasks {
			conn := <-devConn.WriteCH
			buf, err := hex.DecodeString(v.Query)
			if err != nil {
				log.Panicln("[decode err:]: ", err)
				continue
			}
			_, err = conn.Write(buf)

			if err != nil {
				log.Println("[write err]: ", err)
			}

		}

	}
}
