package mqttconn

import (
	"github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"time"
	"fmt"
	"net"
	"net/url"
	"strings"
)

type mqttConn struct {
	url           string
	mqttClient    mqtt.Client
	mqttTopic     string
	readDeadline  time.Time
	writeDeadline time.Time
	readChan      chan []byte
}

func CreateMQTTConnFromURL(uri string) (conn *mqttConn, err error) {
	// Parse uri
	parsedURL, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	mqttProtocol := "tcp"
	portAppend := ""
	switch parsedURL.Scheme {
	case "mqtt":
		mqttProtocol = "tcp"
		if parsedURL.Port() == "" {
			portAppend = ":1883"
		}
	case "mqtts":
		mqttProtocol = "ssl"
	default:
		return nil, errors.New("unrecognized protocol")
	}
	mqttURI := fmt.Sprintf("%s://%s%s", mqttProtocol, parsedURL.Host, portAppend)
	mqttTopic := strings.TrimPrefix(parsedURL.Path, "/")
	user := parsedURL.User
	fmt.Println("mqtturl", mqttURI)

	// build options
	opts := mqtt.NewClientOptions()
	opts.AddBroker(mqttURI)
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	opts.SetClientID(id.String())
	if (user != nil) {
		opts.SetUsername(user.Username())
		password, passwordSet := user.Password()
		if passwordSet {
			opts.SetPassword(password)
		}
	}
	client := mqtt.NewClient(opts)
	token := client.Connect()
	success := token.WaitTimeout(3 * time.Second)
	if !success {
		return nil, errors.New("timed out connecting to server")
	}
	err = token.Error()
	if err != nil {
		return nil, err
	}

	conn, err = CreateMQTTConn(client, mqttTopic)
	conn.url = mqttURI
	return conn, err
}

func CreateMQTTConn(mqttClient mqtt.Client, topic string) (conn *mqttConn, err error) {
	readChan := make(chan []byte, 2)
	mqttClient.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
		readChan <- msg.Payload()
	})
	return &mqttConn{
		mqttClient: mqttClient,
		mqttTopic: topic,
		readChan: readChan,
	}, nil
}

func (conn *mqttConn) Write(p []byte) (n int, err error) {
	token := conn.mqttClient.Publish(conn.mqttTopic, 0, false, p);
	if conn.writeDeadline.IsZero() {
		token.Wait()
	} else {
		waitTime := conn.writeDeadline.Sub(time.Now())
		if waitTime <= 0 {
			return 0, &mqttConnError{true, errors.New("publish timed out")}
		}
		token.WaitTimeout(waitTime)
	}
	err = token.Error()
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (conn *mqttConn) Read(p []byte) (n int, err error) {
	if conn.writeDeadline.IsZero() {
		bytes := <-conn.readChan
		copiedCount := copy(p, bytes)
		return copiedCount, nil
	}
	waitTime := conn.writeDeadline.Sub(time.Now())
	if waitTime <= 0 {
		return 0, &mqttConnError{true, errors.New("read timed out")}
	}
	select {
	case bytes := <-conn.readChan:
		copiedCount := copy(p, bytes)
		return copiedCount, nil
	case <-time.After(waitTime):
		return 0, &mqttConnError{true, errors.New("read timed out")}
	}
}

func (conn *mqttConn) SetDeadline(t time.Time) error {
	conn.readDeadline = t
	conn.writeDeadline = t
	return nil
}

func (conn *mqttConn) SetReadDeadline(t time.Time) error {
	conn.readDeadline = t
	return nil
}

func (conn *mqttConn) SetWriteDeadline(t time.Time) error {
	conn.writeDeadline = t
	return nil
}

func (conn *mqttConn) LocalAddr() net.Addr {
	return mqttAddr("localhost")
}

func (conn *mqttConn) RemoteAddr() net.Addr {
	return mqttAddr(conn.url)
}

type mqttAddr string

func (addr mqttAddr) Network() string {
	return "mqtt"
}

func (addr mqttAddr) String() string {
	return string(addr)
}

type mqttConnError struct {
	isTimeout bool
	err error
}

func (err *mqttConnError) Timeout() bool {
	return err.isTimeout
}

func (err *mqttConnError) Error() string {
	return err.err.Error()
}
