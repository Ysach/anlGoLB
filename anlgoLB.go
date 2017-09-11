package main

import (
	"net"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"encoding/json"
	"os"
	"fmt"
	"crypto/tls"
	"log"
	"github.com/gorilla/websocket"
	"io"
	"strings"
)

type websocketProxy struct {
	Scheme 			string
	WebScheme 		string
	Host 			string
	Path 			string
}

type NewProxy struct {
	Target 		  	[]*url.URL                        `json:"target"`
	Verbose 		bool				  `json:"verbose"`
	SSL 			bool				  `json:"ssl"`
	Port			string				  `json:"port"`
	WebSocketUrl  		websocketProxy
	Dialer   		*websocket.Dialer
	Director 		func(incoming *http.Request, out http.Header)
	Upgrader 		*websocket.Upgrader
}

func main() {
	ProxyStart()
}

func ProxyStart() {
	proxy := &NewProxy{}
	proxy.GetProxy()
	//reverse := NewMultipleHostsReverseProxy(proxy)
	server := &http.Server{
		Addr:                proxy.Port,
		Handler:             proxy,
	}
	//http.ListenAndServe(":3000", proxy.NewMultipleHostsReverseProxy())
	if proxy.SSL == true {
		log.Println("Starting proxy server -- " + proxy.Port)
		err := server.ListenAndServeTLS("keys/cert.pem", "keys/key.pem")
		if err != nil {
			log.Fatal(err)
		}
	}else if proxy.SSL == false {
		log.Println("Starting proxy server -- " + proxy.Port)
		err := server.ListenAndServe()
		if err != nil {
			log.Fatal(err)
		}
	}else {
		log.Fatal("Configuration error, Please set right SSL value  true or false")
	}

}

func (np *NewProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := np.Target[rand.Int() % len(np.Target)]
	if r.Header.Get("Connection") == "Upgrade" || r.Header.Get("Connection") == "upgrade" {
		np.WebSocketUrl.Scheme = target.Scheme
		if target.Scheme == "https" {
			np.WebSocketUrl.WebScheme = "wss://"
		}
		if target.Scheme == "http" {
			np.WebSocketUrl.WebScheme = "ws://"
		}
		np.WebSocketUrl.Host = target.Host
		np.WebSocketUrl.Path = r.RequestURI
		fmt.Println(np.WebSocketUrl)
		np.webSocketProxy(w, r, np.WebSocketUrl)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // 对于自签名的域名证书需要设置skip

	}
	w.Header().Add("User-Agent", "Go-HTTP")
	w.Header().Add("X-Forwarded-Host", r.Host)
	proxy.ServeHTTP(w, r)

}


// get configure info
func (np *NewProxy) GetProxy() {
	file, errs := os.Open("./config.json")
	defer file.Close()
	if errs != nil {
		log.Fatal("config file fail")
		return
	}
	decoder := json.NewDecoder(file)
	err := decoder.Decode(np)
	if err != nil {
		log.Fatal("error:", err)
		return
	}
	get_scheme(np.Target, np.Verbose) // check configure file and get the proxy server scheme
	return
}

// get scheme info
func get_scheme(uRl []*url.URL, verbose bool) {
	scheme_len := len(uRl)
	if scheme_len == 0 {
		log.Fatal("Configuration error， please set your backend proxy server")
	}
	for _, value := range uRl {
		if value.Scheme == "" || value.Host == "" {
			log.Fatal("Configuration error， please set your backend proxy server")
		}
	}
	//if verbose == true {
	for i := 0; i < scheme_len; i ++ {
		if check_scheme(uRl[i].Scheme, uRl) == false && verbose == true{
			log.Fatal("Configuration error，you have different scheme，if you make use is right please set verbose=false")
		}else if verbose == false && check_scheme(uRl[i].Scheme, uRl) == false {
			log.Fatal("Configuration error, you have different scheme")
			return
		}
	}

}

// check scheme
func check_scheme(scheme string, uRl []*url.URL)  bool{
	for _, v := range uRl {
		if scheme != v.Scheme {
			return false
		}
	}
	return true
}

func (np *NewProxy) webSocketProxy(w http.ResponseWriter, r *http.Request, wp websocketProxy) {

	if len(wp.Scheme) == 0 || len(wp.Host) == 0 || len(wp.Path) == 0 {
		log.Println("Not Backend URL Define")
		http.Error(w, "internal server error (code: 1)", http.StatusInternalServerError)
		return
	}

	dialer := np.Dialer
	if np.Dialer == nil {
		dialer = websocket.DefaultDialer
	}

	// header 信息
	requestHeader := http.Header{}
	fmt.Println("Origin----> ", r.Header.Get("Origin"))
	if origin := r.Header.Get("Origin"); origin != "" {
		requestHeader.Add("Origin",wp.Scheme + "://" + wp.Host) // 设置转发主机的地址，否则跨越
	}
	for _, prot := range r.Header[http.CanonicalHeaderKey("Sec-WebSocket-Protocol")] {
		requestHeader.Add("Sec-WebSocket-Protocol", prot)
	}
	for _, cookie := range r.Header[http.CanonicalHeaderKey("Cookie")] {
		requestHeader.Add("Cookie", cookie)
	}

	// RFC7239 请自行查阅
	if clientIP, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if prior, ok := r.Header["X-Forwarded-For"]; ok {
			clientIP = strings.Join(prior, ", ") + ", " + clientIP
		}
		requestHeader.Set("X-Forwarded-For", clientIP)
	}

	// 设置请求源
	requestHeader.Set("X-Forwarded-Proto", "http")
	if r.TLS != nil {
		requestHeader.Set("X-Forwarded-Proto", "https")
	}

	// director 复制
	if np.Director != nil {
		np.Director(r, requestHeader)
	}
	dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	connBackend, resp, err := dialer.Dial(wp.WebScheme + wp.Host + wp.Path, requestHeader)
	if err != nil {
		log.Printf("websocketproxy: couldn't dial to backend url %s\n", err)
		return
	}
	defer connBackend.Close()

	upgrader := np.Upgrader
	if np.Upgrader == nil {
		upgrader = &websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		}
	}

	// Only pass those headers to the upgrader.
	upgradeHeader := http.Header{}
	if hdr := resp.Header.Get("Sec-Websocket-Protocol"); hdr != "" {
		upgradeHeader.Set("Sec-Websocket-Protocol", hdr)
	}
	if hdr := resp.Header.Get("Set-Cookie"); hdr != "" {
		upgradeHeader.Set("Set-Cookie", hdr)
	}

	// Now upgrade the existing incoming request to a WebSocket connection.
	// Also pass the header that we gathered from the Dial handshake.
	connPub, err := upgrader.Upgrade(w, r, upgradeHeader)
	if err != nil {
		log.Printf("websocketproxy: couldn't upgrade %s\n", err)
		return
	}
	defer connPub.Close()

	errc := make(chan error, 2)
	cp := func(dst io.Writer, src io.Reader) {
		_, err := io.Copy(dst, src)
		errc <- err
	}

	// Start our proxy now, everything is ready...
	go cp(connBackend.UnderlyingConn(), connPub.UnderlyingConn())
	go cp(connPub.UnderlyingConn(), connBackend.UnderlyingConn())
	<-errc
	return

}

