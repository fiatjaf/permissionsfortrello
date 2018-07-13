package main

import (
	"crypto/sha256"
	"fmt"

	"github.com/graphql-go/graphql"
)

var queries = graphql.Fields{
	"board": &graphql.Field{
		Type: boardType,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			board := Board{
				Id: p.Context.Value("board").(string),
			}

			err := pg.Get(&board, `
SELECT coalesce(token, '') AS token, enabled FROM boards
WHERE id = $1
            `, board.Id)

			if err != nil {
				log.Warn().Err(err).Str("board", board.Id).Msg("failed to get")
				return nil, err
			}

			return board, nil
		},
	},
}

var mutations = graphql.Fields{
	"initBoard": &graphql.Field{
		Type: resultType,
		Args: graphql.FieldConfigArgument{
			"board": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			boardId := p.Args["board"].(string)

			_, err := pg.Exec("INSERT INTO boards (id) VALUES ($1)", boardId)
			if err != nil {
				return Result{false, "", err.Error()}, nil
			}

			return Result{
				true,
				fmt.Sprintf("%x", sha256.Sum256([]byte(s.SecretKey+":"+boardId))),
				"",
			}, nil
		},
	},
	"setEnabled": &graphql.Field{
		Type: resultType,
		Args: graphql.FieldConfigArgument{
			"token":   &graphql.ArgumentConfig{Type: graphql.String},
			"enabled": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Boolean)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			boardId := p.Context.Value("board").(string)
			token, _ := p.Args["token"].(string)
			enabled := p.Args["enabled"].(bool)

			err := pg.Get(&token, `
UPDATE boards
SET token = CASE WHEN $2 != '' THEN $2 ELSE token END,  
    enabled = $3
WHERE id = $1
RETURNING token
            `, boardId, token, enabled)
			if err != nil {
				log.Warn().Err(err).Str("board", boardId).
					Msg("failed to get token on setEnabled")
				return Result{false, "", err.Error()}, nil
			}

			if enabled {
				// create board webhook
				trello := makeTrelloClient(token)
				err = trello("put", "/1/webhooks", struct {
					CallbackURL string `json:"callbackURL"`
					IdModel     string `json:"idModel"`
				}{s.Host + "/_/webhooks/board", boardId}, nil)
				if err != nil {
					log.Warn().Err(err).Str("board", boardId).
						Msg("failed to create board webhook")
					return nil, err
				}
			}

			return Result{true, "", ""}, nil
		},
	},
}

var boardType = graphql.NewObject(
	graphql.ObjectConfig{
		Name: "board",
		Fields: graphql.Fields{
			"id":        &graphql.Field{Type: graphql.String},
			"shortLink": &graphql.Field{Type: graphql.String},
			"enabled":   &graphql.Field{Type: graphql.Boolean},
			"hasToken": &graphql.Field{
				Type: graphql.Boolean,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					board := p.Source.(Board)
					return board.Token != "", nil
				},
			},
		},
	},
)

var resultType = graphql.NewObject(
	graphql.ObjectConfig{
		Name: "Result",
		Fields: graphql.Fields{
			"ok":    &graphql.Field{Type: graphql.Boolean},
			"value": &graphql.Field{Type: graphql.String},
			"error": &graphql.Field{Type: graphql.String},
		},
	},
)

var rootQuery = graphql.ObjectConfig{Name: "RootQuery", Fields: queries}
var mutation = graphql.ObjectConfig{Name: "Mutation", Fields: mutations}

var schemaConfig = graphql.SchemaConfig{
	Query:    graphql.NewObject(rootQuery),
	Mutation: graphql.NewObject(mutation),
}
