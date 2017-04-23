package main

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"os"

	"github.com/soheilhy/cmux"

	"crypto/tls"

	"net"

	"crypto/x509"

	"golang.org/x/net/http2"
)

func main2() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr:    ":8080", // Normally ":443"
		Handler: http.FileServer(http.Dir(cwd)),
	}
	http2.ConfigureServer(srv, &http2.Server{})
	log.Fatal(srv.ListenAndServeTLS("cert.crt", "cert.key"))
}

func main3() {
	cert, err := tls.LoadX509KeyPair("cert.crt", "cert.key") //tls.X509KeyPair([]byte("cert.crt"), []byte("cert.key"))
	fmt.Println(err)
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS11,
		MaxVersion:   tls.VersionTLS12,
		//NextProtos:   []string{"h2"},
	}

	srv := &http.Server{
		TLSConfig: tlsConf,
		Addr:      ":8080",
		// Handler:   http.FileServer(http.Dir(cwd)),
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hi tester %q\n", html.EscapeString(r.URL.Path))
		w.Write([]byte("done"))

	})

	http2.VerboseLogs = true
	http2.ConfigureServer(srv, &http2.Server{})
	//srv.ListenAndServe()
	srv.ListenAndServeTLS("cert.crt", "cert.key")
}

func main_working() {
	cwd, _ := os.Getwd()
	cert, err := tls.LoadX509KeyPair("cert.crt", "cert.key") //tls.X509KeyPair([]byte("cert.crt"), []byte("cert.key"))
	fmt.Println("cert:", err)
	tlsConf := &tls.Config{
		Certificates:             []tls.Certificate{cert},
		PreferServerCipherSuites: true,
		MinVersion:               tls.VersionTLS11,
		MaxVersion:               tls.VersionTLS12,
		NextProtos:               []string{"h2", "http/1.1"},
	}

	srv := &http.Server{
		TLSConfig:    tlsConf,
		Handler:      http.FileServer(http.Dir(cwd)),
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}

	l, err := tls.Listen("tcp", ":8080", tlsConf)

	http2.VerboseLogs = true
	srv.TLSNextProto["h2"] = h2()

	srv.Serve(l)
}

func h1(l net.Listener) func(s *http.Server, tlsConf *tls.Conn, handler http.Handler) {
	// srv := http2.Server{}
	return func(s *http.Server, conn *tls.Conn, handler http.Handler) {
		s.Serve(l)
		// srv.ServeConn(conn, &http2.ServeConnOpts{
		// 	Handler:    handler,
		// 	BaseConfig: s,
		// })
	}
}

func h2() func(s *http.Server, tlsConf *tls.Conn, handler http.Handler) {
	srv := http2.Server{}
	return func(s *http.Server, conn *tls.Conn, handler http.Handler) {
		log.Println("Using h2")
		srv.ServeConn(conn, &http2.ServeConnOpts{
			Handler:    handler,
			BaseConfig: s,
		})
	}
}

func customHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hi tester %q\n", html.EscapeString(r.URL.Path))
	w.Write([]byte("done"))

}

func main_working_final() {
	//cwd, _ := os.Getwd()
	cert, err := tls.LoadX509KeyPair("cert.crt", "cert.key") //tls.X509KeyPair([]byte("cert.crt"), []byte("cert.key"))
	fmt.Println("cert:", err)
	tlsConf := &tls.Config{
		Certificates:             []tls.Certificate{cert},
		PreferServerCipherSuites: true,
		MinVersion:               tls.VersionTLS11,
		MaxVersion:               tls.VersionTLS12,
		//CipherSuites:             []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
		//NextProtos: []string{"http/1.1", "h2"},
		NextProtos: []string{"h2", "http/1.1"},
		//NextProtos: []string{"h2"},
	}

	//	http.Handle("/", http.FileServer(http.Dir(cwd)))

	l, err := net.Listen("tcp", ":8080")

	http2.VerboseLogs = true

	tcpm := cmux.New(l)
	httpl := tcpm.Match(cmux.HTTP1Fast())

	// tlsl := tcpm.Match(cmux.HTTP2())
	tlsl := tcpm.Match(cmux.Any())

	go serve(httpl, tlsConf)
	// go serveTls(tlsl, tlsConf)
	go serveTls(tlsl, tlsConf)

	tcpm.Serve()
}

func serve(l net.Listener, tlsConf *tls.Config) {
	cwd, _ := os.Getwd()
	srv := &http.Server{
		//TLSConfig:    tlsConf.Clone(),
		Handler: http.FileServer(http.Dir(cwd)),
		//TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}
	//srv.TLSNextProto["h2"] = h2()
	//http2.ConfigureServer(srv2, nil)
	srv.Serve(l)
}

func serveTls(l net.Listener, tlsConf *tls.Config) {
	cwd, _ := os.Getwd()
	srv := &http.Server{
		TLSConfig: tlsConf.Clone(),
		Handler:   http.FileServer(http.Dir(cwd)),
		//TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}
	// srv.TLSNextProto["h2"] = h2()
	// srv.TLSNextProto["http/1.1"] = h2()
	//http2.ConfigureServer(srv, nil)
	//srv.TLSNextProto["http/1.1"] = h1(l)
	//srv1.Serve(l1)

	tlsl := tls.NewListener(l, tlsConf)
	srv.Serve(tlsl)
	// srv.Serve(l)
}

func serveBoth(l1 net.Listener, l2 net.Listener, tlsConf *tls.Config) {
	cwd, _ := os.Getwd()
	// srv1 := &http.Server{
	// 	TLSConfig:    tlsConf.Clone(),
	// 	Handler:      http.FileServer(http.Dir(cwd)),
	// 	TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	// }
	srv2 := &http.Server{
		TLSConfig:    tlsConf.Clone(),
		Handler:      http.FileServer(http.Dir(cwd)),
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}
	//srv1.TLSNextProto["h2"] = h2()
	srv2.TLSNextProto["h2"] = h2()
	//http2.ConfigureServer(srv2, nil)
	//srv.TLSNextProto["http/1.1"] = h1(l)
	//srv1.Serve(l1)
	go srv2.Serve(l2)
}

func main() {
	//cwd, _ := os.Getwd()
	cert, err := tls.LoadX509KeyPair("cert.crt", "cert.key") //tls.X509KeyPair([]byte("cert.crt"), []byte("cert.key"))
	fmt.Println("Err:", err)
	for _, t := range cert.SignedCertificateTimestamps {
		fmt.Println(string(t))
	}

	c, err := x509.ParseCertificate(cert.Certificate[0])
	//fmt.Println(x509.ParseCertificate(cert.Certificate[0]))
	fmt.Println(c.NotAfter)
	fmt.Println(c.NotBefore)
	// fmt.Println(cert.Leaf)
	// fmt.Println(cert.Leaf.NotBefore)
	// fmt.Println(cert.Leaf.NotAfter)

	// for _, t := range cert.Certificate {
	// 	fmt.Println(string(t))
	// }
}
