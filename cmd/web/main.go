package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"database/sql"
	"github.com/alexedwards/scs/redisstore"
	"github.com/alexedwards/scs/v2"
	"github.com/gomodule/redigo/redis"
	_ "github.com/jackc/pgconn"
	_ "github.com/jackc/pgx/v4"
	_ "github.com/jackc/pgx/v4/stdlib"
)

const PORT = "8090"

func main() {
	// connect to the database
	db := initDB()

	// create sessions
	session := initSession()

	// create loggers
	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	errorLog := log.New(os.Stdout, "Error\t", log.Ldate|log.Ltime|log.Lshortfile)
	// create channels

	// create a wait group
	wg := sync.WaitGroup{}
	// set up the application config
	app := Config{
		Session:  session,
		DB:       db,
		Wait:     &wg,
		InfoLog:  infoLog,
		ErrorLog: errorLog,
	}
	// set up mail

	// listen for signals
	go app.listenForShutdown()

	// listen for web connections
	app.serve()
}

func (app *Config) serve() {
	// start http server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", PORT),
		Handler: app.routes(),
	}
	app.InfoLog.Println("starting web server")
	err := srv.ListenAndServe()
	if err != nil {
		log.Panic(err)
	}
}
func initDB() *sql.DB {
	conn := connectToDB()
	if conn == nil {
		log.Panic("cannot connect to database")
	}
	return conn
}

func connectToDB() *sql.DB {
	counts := 0
	dsn := os.Getenv("DSN")

	for {
		conn, err := openDB(dsn)
		if err != nil {
			log.Println("postgres not yet ready...")
		} else {
			log.Println("connected to the database")
			return conn
		}
		if counts > 10 {
			return nil
		}
		log.Println("backing off for 1 second")
		time.Sleep(1 * time.Second)
		counts++

		continue
	}

}
func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		return nil, err
	}
	return db, nil
}

func initSession() *scs.SessionManager {
	// set up session
	session := scs.New()
	session.Store = redisstore.New(initRedis())
	session.Lifetime = 24 * time.Hour
	session.Cookie.Persist = true
	session.Cookie.SameSite = http.SameSiteLaxMode
	session.Cookie.Secure = true

	return session
}

func initRedis() *redis.Pool {
	redisPool := &redis.Pool{
		MaxIdle: 10,
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", os.Getenv("REDIS"))
		},
	}
	return redisPool
}

func (app *Config) listenForShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	app.shutdown()
	os.Exit(0)
}

func (app *Config) shutdown() {
	// perform any clean up tasks
	app.InfoLog.Println("would run clean up tasks")
	// block until wait group is empty
	app.Wait.Wait()

	app.InfoLog.Println("closing channels and shutting down application...")
}
