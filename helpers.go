package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx/types"

	"gopkg.in/jmcvetta/napping.v3"
)

func makeTrelloClient(token string) trelloClient {
	authvalues := (napping.Params{
		"key":   s.TrelloApiKey,
		"token": token,
	}).AsUrlValues()
	h := napping.Session{Params: &authvalues}

	return func(method string, path string, data interface{}, res interface{}) error {
		request := &napping.Request{
			Url:     "https://api.trello.com" + path,
			Method:  method,
			Payload: data,
			Result:  &res,
		}
		n, err := h.Send(request)
		if err != nil || n.Status() > 299 {
			if err == nil {
				err = fmt.Errorf("Trello returned %d for '%s': '%s'",
					n.Status(), n.Url, n.RawText())
			}
			return err
		}
		return nil
	}
}

type trelloClient func(string, string, interface{}, interface{}) error

func userAllowed(trello trelloClient, userId, boardId, cardId string) bool {
	// try admins cache
	if s.RedisURL != "" {
		v, err := rds.Get("admin:" + boardId + ":" + userId).Result()
		if err == nil && v == "t" {
			// the user is a board or team admin
			return true
		}
	}

	// check board and team admins
	var br []struct {
		IdMember      string `json:"idMember"`
		MemberType    string `json:"memberType"`
		OrgMemberType string `json:"orgMemberType"`
	}
	err = trello("get", "/1/boards/"+boardId+"/memberships?member=false&orgMemberType=true", nil, &br)
	if err != nil {
		log.Warn().Str("board", boardId).Err(err).Msg("failed to fetch memberships")
		return false
	}

	for _, ms := range br {
		if ms.IdMember == userId {
			if ms.MemberType == "admin" || ms.OrgMemberType == "admin" {
				return true

				go func() {
					if s.RedisURL != "" {
						rds.Set("admin:"+boardId+":"+userId, "t", time.Hour*2)
					}
				}()
			}
		}
	}

	// check card members
	if cardId == "" {
		// this action was dispatched by something other than a card action
		return false
	}
	var cr []struct {
		Id string `json:"id"`
	}

	err := trello("get", "/1/cards/"+cardId+"/members?fields=id", nil, &cr)
	if err != nil {
		log.Warn().Str("card", cardId).Err(err).
			Msg("failed to fetch memberships")
	} else {
		for _, m := range cr {
			if m.Id == userId {
				return true
			}
		}
	}

	return false
}

func toJSONText(data interface{}) (v types.JSONText, err error) {
	var x []byte
	x, err = json.Marshal(data)
	if err != nil {
		return
	}

	err = v.UnmarshalJSON(x)
	return
}

func saveBackupData(id string, data interface{}) (err error) {
	v, err := toJSONText(data)
	if err != nil {
		return
	}

	_, err = pg.Exec(`
INSERT INTO backups VALUES ($1, $2)
ON CONFLICT (id) DO UPDATE SET data = backups.data || $2
    `, id, v)
	return
}

func updateBackupData(
	id string, initData interface{},
	preupdate, updatefun string, value interface{},
) (err error) {
	d, err := toJSONText(initData)
	if err != nil {
		return
	}

	v, err := toJSONText(value)
	if err != nil {
		return
	}

	updatefun = strings.Replace(
		strings.Replace(updatefun, "$init", "$2", -1),
		"$arg", "$3", -1)

	preupdate = strings.Replace(
		strings.Replace(preupdate, "$init", "$2", -1),
		"$arg", "$3", -1)

	_, err = pg.Exec(`
WITH
ins AS (
  INSERT INTO backups VALUES ($1, $2)
  ON CONFLICT (id) DO NOTHING
),
new AS (
  SELECT (`+preupdate+`) AS data
  FROM backups WHERE id = $1
)
UPDATE backups SET data = `+updatefun+`
FROM new
WHERE id = $1
    `, id, d, v)
	return
}

func fetchBackupData(id string, data interface{}) (err error) {
	var wrapper struct {
		Data types.JSONText `db:"data"`
	}
	err = pg.Get(&wrapper, `SELECT data FROM backups WHERE id = $1`, id)

	if err != nil {
		return
	}

	err = wrapper.Data.Unmarshal(data)
	return
}

func deleteBackupData(id string) (err error) {
	_, err = pg.Exec(`DELETE FROM backups WHERE id = $1`, id)
	return
}
