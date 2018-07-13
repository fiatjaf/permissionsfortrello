package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/graphql-go/graphql"
	"github.com/graphql-go/handler"
	"github.com/jmoiron/sqlx"
	"github.com/kelseyhightower/envconfig"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
	"gopkg.in/redis.v5"
	"gopkg.in/tylerb/graceful.v1"
)

type Settings struct {
	SecretKey       string `envconfig:"SECRET_KEY" required:"true"`
	Host            string `envconfig:"HOST" required:"true"`
	Port            string `envconfig:"PORT" required:"true"`
	PostgresURL     string `envconfig:"DATABASE_URL" required:"true"`
	TrelloApiKey    string `envconfig:"TRELLO_API_KEY" required:"true"`
	TrelloApiSecret string `envconfig:"TRELLO_API_SECRET"`
	RedisURL        string `envconfig:"REDIS_URL"`
}

var err error
var s Settings
var pg *sqlx.DB
var rds *redis.Client
var router *mux.Router
var schema graphql.Schema
var log = zerolog.New(os.Stderr).Output(zerolog.ConsoleWriter{Out: os.Stderr})

func main() {
	err = envconfig.Process("", &s)
	if err != nil {
		log.Fatal().Err(err).Msg("couldn't process envconfig.")
	}

	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	// graphql schema
	schema, err = graphql.NewSchema(schemaConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create graphql schema")
	}
	handler := handler.New(&handler.Config{Schema: &schema})

	// postgres connection
	pg, err = sqlx.Connect("postgres", s.PostgresURL)
	if err != nil {
		log.Fatal().Err(err).Msg("couldn't connect to postgres")
	}

	// redis connection
	if s.RedisURL != "" {
		rurl, _ := url.Parse(s.RedisURL)
		pw, _ := rurl.User.Password()
		rds = redis.NewClient(&redis.Options{
			Addr:     rurl.Host,
			Password: pw,
		})

		if err := rds.Ping().Err(); err != nil {
			log.Fatal().Err(err).Str("url", s.RedisURL).
				Msg("failed to connect to redis")
		}
	}

	// define routes
	router = mux.NewRouter()

	router.Path("/_graphql").Methods("POST").HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			var secret string
			if p := strings.Split(r.Header.Get("Authorization"), " "); len(p) == 2 {
				secret = p[1]
			}

			var board string

			if fmt.Sprintf("%x",
				sha256.Sum256([]byte(s.SecretKey+":"+r.Header.Get("Board"))),
			) == secret {
				board = r.Header.Get("Board")
			}

			ctx := context.WithValue(
				context.TODO(),
				"board", board,
			)

			w.Header().Set("Content-Type", "application/json")

			handler.ContextHandler(ctx, w, r)
		},
	)
	router.Path("/_/webhooks/board").Methods("HEAD").HandlerFunc(ReturnOk)
	router.Path("/_/webhooks/board").Methods("POST").HandlerFunc(handleWebhook)
	router.PathPrefix("/powerup/").Methods("GET").HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")

			if r.URL.Path[len(r.URL.Path)-5:] == ".html" {
				http.ServeFile(w, r, "./powerup/basic.html")
				return
			}

			if r.URL.Path == "/powerup/icon.svg" {
				color := "#" + r.URL.Query().Get("color")
				if color == "#" {
					color = "#999999"
				}
				w.Header().Set("Content-Type", "image/svg+xml")
				http.ServeFile(w, r, "icon.svg")
				return
			}

			http.ServeFile(w, r, "."+r.URL.Path)
		},
	)

	router.Path("/favicon.ico").Methods("GET").HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, "./powerup/icon.png")
			return
		})

	// start the server
	log.Info().Str("port", os.Getenv("PORT")).Msg("listening.")
	graceful.Run(":"+os.Getenv("PORT"), 10*time.Second, router)
}

func ReturnOk(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
}
