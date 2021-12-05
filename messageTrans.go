package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)
var Tran chan []byte
var wg sync.WaitGroup
var flag1 int64 = 0
var errUnexpectedRead = errors.New("unexpected read from socket")
var Serverconn net.Conn
var Rwlock sync.RWMutex
var ServerIP string
var ServerPort string
var ClientIP string
var ClientPort string

//connectServer
/*
*连接远程服务器，开启接收服务器消息线程
*/
func connectServer(ip string,port string) (flag bool){

	var err error
	for {
		connTimeout := 4*time.Second
		Rwlock.Lock()
		Serverconn,err = net.DialTimeout("tcp",ip+":"+port,connTimeout)
		Rwlock.Unlock()
		if err != nil {
			fmt.Println("failed to connect Server:"+ip+":"+port)
			fmt.Println("waiting 500 Microseconds")
			time.Sleep(500*time.Microsecond)
		}else {
			fmt.Println("successful to connect server",ip+":"+port)
			Serverconn.Write([]byte("connect\r\n"))
			break

		}
	}
	go receiverMessage()
	wg.Add(1)
	return true



}
//检查错误函数
func connCheck(conn net.Conn) error {
	var sysErr error

	sysConn, ok := conn.(syscall.Conn)
	if !ok {
		return nil
	}
	rawConn, err := sysConn.SyscallConn()
	if err != nil {
		return err
	}

	err = rawConn.Read(func(fd uintptr) bool {
		var buf [1]byte
		n, err := syscall.Read(syscall.Handle(fd), buf[:])
		switch {
		case n == 0 && err == nil:
			sysErr = io.EOF
		case n > 0:
			sysErr = errUnexpectedRead
		case err == syscall.EAGAIN || err == syscall.EWOULDBLOCK:
			sysErr = nil
		default:
			sysErr = err
		}
		return true
	})
	if err != nil {
		return err
	}
	return sysErr
}
/*connectPrinter
*连接打印机，并开启监听管道线程，将管道数据发送给打印机
*/
func connectPrinter(ip string,port string) (flag bool) {
	var conn net.Conn
	var err error
	for {
		connTimeout := 2*time.Second
		conn,err = net.DialTimeout("tcp",ip+":"+port,connTimeout)
		if err != nil {
			fmt.Println("error to connect Printer:"+ip+":"+port)
			fmt.Println("waiting 500 Microseconds")
			time.Sleep(500*time.Microsecond)

		}else {
			fmt.Println("successful to connect printer client",ip+":"+port)
			break
		}
	}
	/*for {
		select {
		case <-Tran:
		default:
			break
		}
	}*/
	atomic.StoreInt64(&flag1,0)
	go sendToPtinter(conn)
	wg.Add(1)
	return true

}

// HeartBeat
/*
*心跳线程每10秒发送心跳消息至打印机和服务器，
*检测连接是否正常，防止连接出现异常，
*/
func HeartBeat() {
	for {
		Tran <- []byte("heartbeat\r\n")
		time.Sleep(10 * time.Second)
		if Serverconn != nil {
			Rwlock.RLock()
			n,err:=Serverconn.Write([]byte("heartbeat\r\n"))
			fmt.Println(time.Now().Format("2006-01-02 15:04:05"),"send heartbeat to server================>>:",n,"bytes")
			Rwlock.RUnlock()
			if err != nil {
				fmt.Println("Server socket is closed")
				Serverconn.Close()
				Rwlock.Lock()
				Serverconn = nil
				Rwlock.Unlock()
			}
		}

	}
}
//sendToPtinter
/*
*读取管道数据，将数据发送至打印机
*
*/
func sendToPtinter(conn net.Conn) {
	defer conn.Close()
	for {
		buf := <-Tran
		fmt.Println(time.Now().Format("2006-01-02 15:04:05"), " Send to print message======================>>:", len(buf), "bytes")
		_, err := conn.Write(buf)
		if err != nil {
			fmt.Println("send error")
			atomic.StoreInt64(&flag1, 1)
			wg.Done()
			break
		}
		 time.Sleep(100*time.Microsecond)

	}
	connectPrinter(ClientIP,ClientPort)
	//connectPrinter("127.0.0.1","9000")

}
//receiverMessage
/*
*接收服务器消息线程，并写入管道Tran
*/
func receiverMessage() {
	buf := [4096]byte{}
	var send = []byte{}
	for {
		if Serverconn == nil {
			break
		}
		Rwlock.RLock()
		n, err := Serverconn.Read(buf[0:])
		Rwlock.RUnlock()
		if err != nil {
			fmt.Println("receive error")
			Serverconn.Close()
			Rwlock.Lock()
			Serverconn = nil
			Rwlock.Unlock()
			wg.Done()
			break
		} else if n == 4096 {    //接收超过4096个字节
			send = append(send,buf[0:n]...)
			//Tran <- buf[0:n]
		} else {
			send = append(send, buf[0:n]...)
			fmt.Println(time.Now().Format("2006-01-02 15:04:05")," Receive remote server message=================>>:", len(send[:]),"bytes")
			Tran <- send
			send = nil
		}
		time.Sleep(100*time.Microsecond)
	}
	connectServer(ServerIP,ServerPort)
	//connectServer("127.0.0.1","8888")
}
//receiveTran
/*
*防止客户端断开后，管道阻塞
*/
func receiveTran() {
	for {
		if atomic.LoadInt64(&flag1) == 1 {
			 buf := <-Tran
			 fmt.Println(buf)

		}
		time.Sleep(500*time.Microsecond)
	}
}

func main() {
	Tran = make(chan []byte)
	go receiveTran()
	fi, err := os.Open("./interconnect.conf")
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}
	defer fi.Close()
	br := bufio.NewReader(fi)
	addr,_,_ := br.ReadLine()
	str := strings.Split(string(addr),":")
	ServerIP = str[0]
	ServerPort = str[1]
	ClientIP = str[2]
	ClientPort = str[3]
	for i:=0;i<4;i++ {
		fmt.Println(str[i])
	}
	connectServer(ServerIP,ServerPort)
	connectPrinter(ClientIP,ClientPort)
	go HeartBeat()
	wg.Wait()
	close(Tran)
}
