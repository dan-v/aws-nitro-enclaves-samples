package main

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/mdlayher/vsock"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	defaultPort = 5005
	defaultURL = "http://127.0.0.1:8080"
	defaultProxyAddress = "127.0.0.1:8080"
)

var (
	cid, clientPort, serverListenPort uint32
	clientURL, serverProxyAddress string
	clientDebug, serverDebug bool
)

func init() {
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(clientCmd)
	clientCmd.Flags().Uint32VarP(&cid, "cid", "c", 0, "CID")
	clientCmd.Flags().Uint32VarP(&clientPort, "vsock-port", "v", defaultPort, "Remote Port")
	clientCmd.Flags().StringVarP(&clientURL, "url", "u", defaultURL, "URL to request inside enclave")
	clientCmd.Flags().BoolVarP(&clientDebug, "debug", "d", false, "Debug mode uses TCP rather than vsock")
	clientCmd.MarkFlagRequired("cid")
	serverCmd.Flags().Uint32VarP(&serverListenPort, "listen-port", "l", defaultPort, "Listen Port")
	serverCmd.Flags().StringVarP(&serverProxyAddress, "proxy-address", "p", defaultProxyAddress, "Address inside enclave to proxy connection to")
	serverCmd.Flags().BoolVarP(&serverDebug, "debug", "d", false, "Debug mode uses TCP rather than vsock")
}

var rootCmd = &cobra.Command{
	Use:   "vsocksample",
	Short: "vsock example in go showing http client and server",
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "run http server with vsock listener",
	Run: func(cmd *cobra.Command, args []string) {
		go func() {
			log.Printf("starting example server on %v", defaultProxyAddress)
			tcpListener, err := net.Listen("tcp", defaultProxyAddress)
			if err != nil {
				log.Fatal(err)
			}
			mux := http.NewServeMux()
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, "testing tcp server behind vsocks proxy\n")
			})
			log.Fatal(http.Serve(tcpListener, mux))
		}()

		var l net.Listener
		var err error
		if !serverDebug {
			log.Printf("starting vsock proxy on port %v\n", serverListenPort)
			l, err = vsock.Listen(serverListenPort)
			if err != nil {
				log.Fatal(err)
			}
		} else {
			log.Printf("starting debug tcp proxy on port %v\n", serverListenPort)
			l, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%v", serverListenPort))
			if err != nil {
				log.Fatal(err)
			}
		}
		defer l.Close()

		for {
			log.Println("waiting for new client connection")
			c, err := l.Accept()
			if err != nil {
				fmt.Println(err)
				return
			}
			log.Println("accepted connection")
			go handleConnection(c)
		}
	},
}

func handleConnection(client net.Conn) error {
	defer client.Close()

	target, err := net.Dial("tcp", serverProxyAddress)
	if err != nil {
		return fmt.Errorf("failed to connect to target: %w", err)
	}
	defer target.Close()
	log.Printf("connected to proxy %v\n", serverProxyAddress)

	log.Printf("proxying traffic from client to %v\n", serverProxyAddress)
	close := sync.Once{}
	go copy(client, target, close)
	err = copy(target, client, close)
	if err != nil {
		return err
	}
	log.Printf("proxying traffic completed")

	return nil
}

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "run http client over vsock connection",
	Run: func(cmd *cobra.Command, args []string) {
		tr := &http.Transport{
			IdleConnTimeout:    15 * time.Second,
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				if clientDebug {
					log.Printf("client connecting to debug tcp port=%v\n", clientPort)
					return net.Dial("tcp", fmt.Sprintf("127.0.0.1:%v", clientPort))
				}
				log.Printf("client connecting to vsock cid=%v port=%v\n", cid, clientPort)
				return vsock.Dial(cid, clientPort)
			},
		}
		client := &http.Client{Transport: tr}
		resp, err := client.Get(clientURL)
		if err != nil {
			log.Fatal(err)
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}
		log.Println("received from server: ", string(body))
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}


func copy(dst io.WriteCloser, src io.ReadCloser, close sync.Once) error {
	_, err := io.Copy(dst, src)
	if err != nil {
		return err
	}
	close.Do(func() {
		dst.Close()
		src.Close()
	})
	return nil
}