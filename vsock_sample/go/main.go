package main

import (
	"context"
	"fmt"
	"github.com/mdlayher/vsock"
	"github.com/spf13/cobra"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"
)

var (
	cid, clientPort, serverPort uint32
)

func init() {
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(clientCmd)
	clientCmd.Flags().Uint32VarP(&cid, "cid", "c", 0, "CID")
	clientCmd.Flags().Uint32VarP(&clientPort, "port", "p", 0, "Remote Port")
	clientCmd.MarkFlagRequired("cid")
	clientCmd.MarkFlagRequired("port")
	serverCmd.Flags().Uint32VarP(&serverPort, "port", "p", 0, "Listen Port")
	serverCmd.MarkFlagRequired("port")
}

var rootCmd = &cobra.Command{
	Use:   "vsocksample",
	Short: "vsock example in go showing http client and server",
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "run http server with vsock listener",
	Run: func(cmd *cobra.Command, args []string) {
		log.Printf("starting server on port %v\n", serverPort)
		l, err := vsock.Listen(serverPort)
		if err != nil {
			log.Fatal(err)
		}
		defer l.Close()

		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "testing http with vsock\n")
		})
		log.Fatal(http.Serve(l, mux))
	},
}

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "run http client over vsock connection",
	Run: func(cmd *cobra.Command, args []string) {
		log.Printf("client connecting to cid=%v port=%v\n", cid, clientPort)
		tr := &http.Transport{
			IdleConnTimeout:    15 * time.Second,
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return vsock.Dial(cid, clientPort)
			},
		}
		client := &http.Client{Transport: tr}
		resp, err := client.Get("http://test/")
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

