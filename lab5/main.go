package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"
)

type resolveResult struct {
	ip  string
	err error
}

func main() {
	port := "1080"

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	defer listener.Close()

	fmt.Println("Сервер запущен на порту ", port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Fatal(err)
			continue
		}

		go handleClient(conn)
	}
}

func handleClient(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 512)
	_, err := conn.Read(buf)
	if err != nil {
		log.Fatal(err)
		return
	}

	if buf[0] != 0x05 {
		fmt.Println("Неверная версия SOCKS: ", buf[0])
		return
	}

	conn.Write([]byte{0x05, 0x00})

	_, err = conn.Read(buf)
	if err != nil {
		log.Fatal(err)
		return
	}

	if buf[1] != 0x01 {
		fmt.Println("Поддерживается только команда CONNECT")
		conn.Write([]byte{0x05, 0x07})
		return
	}

	var targetAddr string
	switch buf[3] {
	case 0x01:
		targetAddr = fmt.Sprintf("%d.%d.%d.%d:%d", buf[4], buf[5], buf[6], buf[7], int(buf[8])<<8|int(buf[9]))
	case 0x03:
		domainLen := int(buf[4])
		domain := string(buf[5 : 5+domainLen])

		resultChan := make(chan resolveResult)
		asyncResolveDomain(domain, resultChan)

		select {
		case result := <-resultChan:
			if result.err != nil {
				fmt.Println("Ошибка резолвинга: ", result.err)
				conn.Write([]byte{0x05, 0x04})
				return
			}

			targetAddr = fmt.Sprintf("%s:%d", result.ip, int(buf[5+domainLen])<<8|int(buf[6+domainLen]))
			fmt.Println("Резолвинг успешен, IP-адрес: ", targetAddr)

		case <-time.After(5 * time.Second):
			fmt.Println("Таймаут резолвинга")
			conn.Write([]byte{0x05, 0x04})
			return
		}
	case 0x04:
		ip := net.IP(buf[4 : 4+16])
		port := int(buf[20])<<8 | int(buf[21])
		targetAddr = fmt.Sprintf("[%s]:%d", ip.String(), port)
	default:
		fmt.Println("Тип адреса не поддерживается: ", buf[3])
		conn.Write([]byte{0x05, 0x08})
		return
	}

	fmt.Println("Соединение с ", targetAddr)

	targetConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		log.Fatal(err)
		conn.Write([]byte{0x05, 0x05})
		return
	}

	defer targetConn.Close()

	if err := sendSuccessResponse(conn, buf); err != nil {
		fmt.Println("Ошибка отправки ответа клиенту: ", err)
		conn.Write([]byte{0x05, 0x05})
		return
	}
	go proxyData(conn, targetConn, "client -> target")
	proxyData(targetConn, conn, "target -> client")
}

func sendSuccessResponse(conn net.Conn, reqBuf []byte) error {
	var response []byte
	switch reqBuf[3] {
	case 0x01:
		response = []byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}
	case 0x03:
		response = []byte{0x05, 0x00, 0x00, 0x03, reqBuf[4]}
		response = append(response, reqBuf[5:5+int(reqBuf[4])]...)
		response = append(response, 0, 0)
	case 0x04:
		response = []byte{0x05, 0x00, 0x00, 0x04}
		response = append(response, reqBuf[4:4+16]...)
		response = append(response, 0, 0)
	default:
		return fmt.Errorf("Неподдерживаемый тип адреса")
	}

	_, err := conn.Write(response)
	return err
}
func proxyData(src, dest net.Conn, direction string) {
	_, err := io.Copy(dest, src)
	if err != nil {
		fmt.Printf("Ошибка перенаправления данных (%s): %v\n", direction, err)
	}
}
func asyncResolveDomain(domain string, resultChan chan<- resolveResult) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		ips, err := net.DefaultResolver.LookupHost(ctx, domain)
		if err != nil || len(ips) == 0 {
			resultChan <- resolveResult{"", fmt.Errorf("не удалось разрешить доменное имя: %v", err)}
			return
		}

		resultChan <- resolveResult{ips[0], nil}
	}()
}
