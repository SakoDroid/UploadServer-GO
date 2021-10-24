package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/textproto"
	"os"
	"regexp"
	"strconv"
)

var accessLog *log.Logger
var internalLogs *log.Logger
var port = 8080
var contentTypes = map[string]string{".html" : "text/html",".css" : "text/css",".js" : "application/json","" : "text/plain"}

func main(){
	initTheLoggers()
	mkFileDir()
	internalLogs.Println("Starting the server ...")
	mux := http.NewServeMux()
	configTheHandlers(mux)
	doneChannel := make(chan bool)
	go startTheServer(mux,&doneChannel)
	fmt.Println("Server running on port",port,"...")
	internalLogs.Println("Server started, listening on port",port)
	<- doneChannel
}

func mkFileDir(){
	_,err := os.Stat("./files")
	if err != nil{
		err2 := os.Mkdir("./files",0666)
		if err2 != nil{
			internalLogs.Println("Could not check the Files dir : ",err)
			panic("Could not check the Files dir")
		}
	}
	internalLogs.Println("Files directory checked.")
}

func startTheServer(mux *http.ServeMux,doneChannel *chan bool){
	err := http.ListenAndServe("localhost:8080",mux)
	*doneChannel <- true
	if err != nil{
		internalLogs.Println("Could not start the server : ",err)
		panic("Could not start the server")
	}
}

func initTheLoggers(){
	accessLogFile,err := os.OpenFile("access.log",os.O_CREATE | os.O_APPEND | os.O_RDWR,0666)
	internalLogFile,err2 := os.OpenFile("server-logs.log",os.O_CREATE | os.O_APPEND | os.O_RDWR,0666)
	if err == nil && err2 == nil{
		accessLog = log.New(accessLogFile,"",log.Ltime | log.Ldate)
		internalLogs = log.New(internalLogFile,"",log.Ldate | log.Ltime | log.Llongfile)
		internalLogs.Println("--------------------------------------------\nLoggers initialized")
	}else{
		fmt.Println(err)
		fmt.Println(err2)
		panic("Error occurred in loggers initialization")
	}
}

func configTheHandlers(mux *http.ServeMux){
	mux.HandleFunc("/",mainDir)
	mux.HandleFunc("/upload",upload)
	mux.HandleFunc("/files",fileAPI)
	internalLogs.Println("Handlers have been configured")
}

func mainDir(wr http.ResponseWriter,req *http.Request){
	path := req.URL.Path
	if path == "/"{
		sendAFile(&wr,"index.html",req,false)
	}else{
		sendAFile(&wr,"." + path,req,false)
	}
}

func upload(wr http.ResponseWriter,req *http.Request){
	err := req.ParseMultipartForm(1024)
	if err == nil {
		htmlResponse := "<html><head><title>Upload successful!</title></head><body><h1>Files uploaded!</h1>" +
			"<h3>Click the links below for the files!</h3><br/>"
		i := 0
		for key := range req.MultipartForm.File{
			i++
			ft,header,err2 := req.FormFile(key)
			if err2 == nil{
				fs,err3 := os.OpenFile("./files/" + header.Filename,os.O_CREATE | os.O_RDWR,0666)
				if err3 == nil{
					_, err4 := io.Copy(fs,ft)
					if err4 == nil {
						htmlResponse += "<span>" + strconv.Itoa(i) + "-  </span><a href=\"/files/" + header.Filename + "\">" + header.Filename  +"</a><br/><hr/>"
					}else{
						sendError(&wr,500,"",req.RemoteAddr)
						internalLogs.Println("Problem in copying file : ",err4)
					}
				}else{
					sendError(&wr,500,"",req.RemoteAddr)
					internalLogs.Println("Problem in creating file : ",err3)
				}
			}else{
				sendError(&wr,500,"",req.RemoteAddr)
				internalLogs.Println("Problem in fetching file : ",err2)
			}
		}
		wr.Header().Set(textproto.CanonicalMIMEHeaderKey("content-type"),"text/html")
		wr.WriteHeader(200)
		_,er := wr.Write([]byte(htmlResponse + "</body></html>"))
		if er != nil{
			sendError(&wr,500,req.URL.Path,req.RemoteAddr)
			internalLogs.Println(req.RemoteAddr," -> ",req.URL.Path," -> ",er)
		}else{
			accessLog.Println(req.RemoteAddr," -> ",req.URL.Path," -> 200")
		}
	}else{
		sendError(&wr,500,"",req.RemoteAddr)
		internalLogs.Println("Problem in parsing form : ",err)
	}
}

func fileAPI(wr http.ResponseWriter,req *http.Request){
	if req.URL.Path == "/files"{
		response := "<html><head><title>Files list</title></head><body><h1>Files list</h1><hr/>"
		filesDir,err := os.Open("./files")
		if err == nil{
			dirList,_ := filesDir.Readdir(0)
			for ind,fInfo := range dirList{
				response += "<span>" + strconv.Itoa(ind + 1) + "-  </span><a href=\"/files/" + fInfo.Name() + "\">" + fInfo.Name() +"</a><br/><hr/>"
			}
			wr.Header().Set(textproto.CanonicalMIMEHeaderKey("content-type"),"text/html")
			wr.WriteHeader(200)
			_, err2 := wr.Write([]byte(response + "</body></html>"))
			if err2 != nil {
				sendError(&wr,500,req.URL.Path,req.RemoteAddr)
			}else{
				accessLog.Println(req.RemoteAddr," -> ",req.URL.Path," -> 200")
			}
		}else{
			sendError(&wr,500,req.URL.Path,req.RemoteAddr)
			internalLogs.Println("Unable to open Files dir : ",err)
		}
	}else{
		sendAFile(&wr,"." + req.URL.Path,req,true)
	}
}

func sendAFile(wr *http.ResponseWriter,path string,req * http.Request,httpServ bool){
	clientAddress := req.RemoteAddr
	_,err := os.Stat(path)
	if err == nil{
		if !httpServ{
			(*wr).Header().Set(textproto.CanonicalMIMEHeaderKey("content-type"), contentTypes[getFileExt(path)])
			(*wr).WriteHeader(200)
			file, err2 := os.Open(path)
			if err2 == nil {
				_, err3 := io.Copy(*wr, file)
				if err3 == nil {
					accessLog.Println(clientAddress, " -> ", path, " -> ", "200")
				} else {
					sendError(wr, 500, path, clientAddress)
				}
			} else {
				sendError(wr, 500, path, clientAddress)
				internalLogs.Println(clientAddress, " -> ", path, " ->  Error in opening file")
			}
		}else{
			http.ServeFile(*wr,req,path)
		}
	}else{
		sendError(wr,404,path,clientAddress)
	}
}

func sendError(wr *http.ResponseWriter,code int,path,clientAddress string){
	(*wr).WriteHeader(code)
	_, err2 := (*wr).Write([]byte(strconv.Itoa(code) + "!"))
	if err2 != nil {
		accessLog.Println(clientAddress," -> ",path," -> ","500")
		internalLogs.Println(clientAddress," -> ",path," ->  Failed to send ",code)
	}else{
		accessLog.Println(clientAddress," -> ",path," -> ",code)
	}
}

func getFileExt(name string) string{
	reg, _ := regexp.Compile("[.]\\w+$")
	return reg.FindString(name)
}
