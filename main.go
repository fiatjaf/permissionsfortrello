package main

import (
	"net/http"
	"net/url"
	"os"
	"text/template"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/graphql-go/graphql"
	"github.com/jmoiron/sqlx"
	"github.com/kelseyhightower/envconfig"
	_ "github.com/lib/pq"
	"github.com/minio/minio-go"
	"github.com/mrjones/oauth"
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
	AWSKeyId        string `envconfig:"AWS_KEY_ID" required:"true"`
	AWSSecretKey    string `envconfig:"AWS_SECRET_KEY" required:"true"`
	S3BucketName    string `envconfig:"S3_BUCKET_NAME" required:"true"`
}

var err error
var s Settings
var c *oauth.Consumer
var pg *sqlx.DB
var rds *redis.Client
var ms3 *minio.Client
var tmpl *template.Template
var store sessions.Store
var router *mux.Router
var schema graphql.Schema
var log = zerolog.New(os.Stderr).Output(zerolog.ConsoleWriter{Out: os.Stderr})

func main() {
	err = envconfig.Process("", &s)
	if err != nil {
		log.Fatal().Err(err).Msg("couldn't process envconfig.")
	}

	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	log = log.With().Timestamp().Logger()

	// cookie store
	store = sessions.NewCookieStore([]byte(s.SecretKey))

	// minio s3 client
	ms3, _ = minio.New(
		"s3.amazonaws.com",
		s.AWSKeyId,
		s.AWSSecretKey,
		true,
	)

	// templates
	tmpl = template.Must(template.New("~").ParseGlob("templates/*.html"))

	// oauth consumer
	c = oauth.NewConsumer(
		s.TrelloApiKey,
		s.TrelloApiSecret,
		oauth.ServiceProvider{
			RequestTokenUrl:   "https://trello.com/1/OAuthGetRequestToken",
			AuthorizeTokenUrl: "https://trello.com/1/OAuthAuthorizeToken",
			AccessTokenUrl:    "https://trello.com/1/OAuthGetAccessToken",
		},
	)
	c.AdditionalAuthorizationUrlParams["name"] = "Permissions for Trello"
	c.AdditionalAuthorizationUrlParams["scope"] = "read,write,account"
	c.AdditionalAuthorizationUrlParams["expiration"] = "never"

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

	router.Path("/").Methods("GET").HandlerFunc(ServeIndex)
	router.Path("/auth").Methods("GET").HandlerFunc(TrelloAuth)
	router.Path("/auth/callback").Methods("GET").HandlerFunc(TrelloAuthCallback)
	router.Path("/account").Methods("GET").HandlerFunc(ServeAccount)
	router.Path("/setBoard").Methods("POST").HandlerFunc(handleSetupBoard)
	router.Path("/_/webhooks/board").Methods("HEAD").HandlerFunc(returnOk)
	router.Path("/_/webhooks/board").Methods("POST").HandlerFunc(handleWebhook)
	router.PathPrefix("/public/").Methods("GET").Handler(
		http.FileServer(http.Dir(".")),
	)
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
			http.ServeFile(w, r, "./public/icon.png")
			return
		})

	// start the server
	log.Info().Str("port", os.Getenv("PORT")).Msg("listening.")
	graceful.Run(":"+os.Getenv("PORT"), 10*time.Second, router)
}
