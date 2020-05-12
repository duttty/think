package scheduler

import (
	"fmt"
	"log"

	"think/models"
	mqclient "think/mqtt/client"

	"net"
	"time"
)

type DevConn struct {
	// 兼容半双工设备
	WriteCH   chan net.Conn
	DevID     string
	MessageID uint64
	PointID   uint64
	Conn      net.Conn
	Status    uint8
}

var ID2DevConnMap = make(map[string]*DevConn)

// ConnClose 关闭连接，删除设备，关闭套接字
func (c *DevConn) ConnClose() {
	// 从map移除设备
	delete(ID2DevConnMap, c.DevID)
	close(c.WriteCH)

	// 关闭Conn
	c.Conn.Close()
}
func (c *DevConn) Online() {
	// 写入map
	ID2DevConnMap[c.DevID] = c
	log.Printf("[online]--devID:%s", c.DevID)
	c.Conn.Write([]byte("0001"))
	// 修改数据库设备状态
	device := models.DeviceStatus(c.DevID, 1)
	// 设备存在
	if device.ID != 0 {
		// 加载定时任务
		task := models.GetDevSchedule(c.DevID)
		TaskCH <- task

		// 设备上线MQTT推送
		tpc := fmt.Sprintf("%s/device", device.Username)
		mqclient.Client.Publish(tpc, 0, false, device)
	}

}

func (c *DevConn) Offline() {
	log.Printf("[offline]--devID:%s Addr:%s\n", c.DevID, c.Conn.RemoteAddr())
	// 关闭连接
	c.ConnClose()
	// 修改设备状态
	device := models.DeviceStatus(c.DevID, 0)
	// 设备存在
	if device.ID != 0 {
		// 停止定时任务
		stop := StopMap.Get(c.DevID)
		if stop != nil {
			stop <- true
		}
		// 设备上线MQTT推送
		tpc := fmt.Sprintf("%s/device", device.Username)
		mqclient.Client.Publish(tpc, 0, false, device)
	}
}

func (c *DevConn) SendConn() {
	select {
	case c.WriteCH <- c.Conn:
		log.Println("[send ok]")
	default:
	}
}

func receiveTCP(addr string) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	for {
		conn, err := l.Accept()
		if err != nil {
			continue
		}
		log.Printf("[recvConn] : %s", conn.RemoteAddr())
		// 创建写管道
		ch := make(chan net.Conn, 1)
		ch <- conn
		go handler(&DevConn{Conn: conn, WriteCH: ch})
	}
}

// 试图读取设备ID
func handler(devConn *DevConn) {
	data := make([]byte, 32)
	// 读取数据
	for {
		devConn.Conn.SetReadDeadline(time.Now().Add(time.Second * 3))
		n, err := devConn.Conn.Read(data)
		if err != nil {
			// 是否为Timeout
			if e, ok := err.(net.Error); ok && e.Timeout() {
				devConn.SendConn()
				continue
			}
			// 设备下线
			devConn.Offline()
			log.Printf("[err]--%s", err)
			return
		}
		dec := string(data[:n])
		hex := fmt.Sprintf("%x", data[:n])
		// 判断是否为注册包
		if len(dec) == 8 && dec[:2] == "80" {
			// 判断设备是否存在
			old, ok := ID2DevConnMap[dec]
			if ok {
				// 设备存在
				log.Printf("[err]--devID:%s used addr:%s ", old.DevID, old.Conn.RemoteAddr())
				devConn.Conn.Write([]byte("0000"))
				devConn.ConnClose()
				return
			}
			// 添加设备ID
			devConn.DevID = dec

			// 通知设备上线
			devConn.Online()

			continue
		}
		if len(hex) == 8 && hex[:2] == "80" {
			old, ok := ID2DevConnMap[hex]
			if ok {
				// 设备存在
				log.Printf("[err]--devID:%s used addr:%s ", old.DevID, old.Conn.RemoteAddr())
				devConn.Conn.Write([]byte("0000"))
				devConn.ConnClose()
				return
			}
			// 添加设备ID
			devConn.DevID = hex

			// 通知设备上线
			devConn.Online()
			continue
		}
		// 从机消息
		tpc := fmt.Sprintf("dt/%s", devConn.DevID)
		if devConn.DevID == "" {
			log.Println("[unsign dev data]:", hex)
			continue
		}
		// 定时任务消息
		if devConn.PointID != 0 {
			// 存入数据库
			models.SavePointData(devConn.PointID, devConn.DevID, hex)
			devConn.SendConn()
		}

		mqclient.Client.Publish(fmt.Sprintf("%s/data", tpc), 0, false, hex)
		devConn.SendConn()
		log.Println("[recv data :]", hex)
	}
}
