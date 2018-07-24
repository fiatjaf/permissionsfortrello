package main

import (
	"database/sql"
	"net/http"
	"net/url"
	"strings"

	"github.com/mrjones/oauth"
	"gopkg.in/jmcvetta/napping.v3"
)

func ServeIndex(w http.ResponseWriter, r *http.Request) {
	err = tmpl.ExecuteTemplate(w, "index.html", nil)
	if err != nil {
		log.Warn().Err(err).Msg("failed to render /")
	}
}

func TrelloAuth(w http.ResponseWriter, r *http.Request) {
	sess, _ := store.Get(r, "auth-session")
	if v, ok := sess.Values["id"]; ok && v != "" {
		// already logged
		http.Redirect(w, r, "/account", http.StatusFound)
		return
	}

	token, url, err := c.GetRequestTokenAndUrl("http://" + r.Host + "/auth/callback")
	if err != nil {
		log.Print(err)
		http.Error(w, "Couldn't redirect you to Trello.", 503)
		return
	}

	sess.Values[token.Token] = token.Secret
	sess.Save(r, w)

	http.Redirect(w, r, url, http.StatusFound)
}

func TrelloAuthCallback(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	verificationCode := values.Get("oauth_verifier")
	tokenKey := values.Get("oauth_token")

	sess, _ := store.Get(r, "auth-session")
	tokenI := sess.Values[tokenKey]
	if tokenI == nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	requestToken := &oauth.RequestToken{
		tokenKey,
		tokenI.(string),
	}

	accessToken, err := c.AuthorizeToken(requestToken, verificationCode)
	if err != nil {
		http.Error(w, "Invalid token, did something went wrong on your Trello login?", 401)
		return
	}

	var profile struct {
		Id       string `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	response, err := napping.Get("https://trello.com/1/members/me", &url.Values{
		"key":    []string{s.TrelloApiKey},
		"token":  []string{accessToken.Token},
		"fields": []string{"username,id,email"},
	}, &profile, nil)
	if err != nil || response.Status() > 299 {
		http.Error(w, "Failed to fetch your profile info from Trello. This is odd.", 503)
		return
	}

	delete(sess.Values, tokenKey)
	sess.Values["id"] = profile.Id
	sess.Values["username"] = profile.Username
	sess.Values["email"] = profile.Email
	sess.Values["token"] = accessToken.Token
	sess.Save(r, w)

	http.Redirect(w, r, "/account", http.StatusFound)
}

func ServeAccount(w http.ResponseWriter, r *http.Request) {
	sess, _ := store.Get(r, "auth-session")
	username, ok1 := sess.Values["username"]
	token, ok2 := sess.Values["token"]
	email, ok3 := sess.Values["email"]
	if !ok1 || !ok2 || !ok3 {
		http.Redirect(w, r, "/auth", http.StatusFound)
		return
	}

	trello := makeTrelloClient(token.(string))

	// get all boards for which this user is an admin
	var allboards []Board
	err := trello("get",
		"/1/members/"+username.(string)+
			"/boards?filter=open&fields=id,shortLink,name,memberships&memberships=me",
		nil, &allboards)
	if err != nil {
		http.Error(w, "failed to fetch trello boards: "+err.Error(), 503)
		return
	}

	var boards []Board
	for _, board := range allboards {
		m := board.Memberships[0]
		if m.MemberType == "admin" || m.OrgMemberType == "admin" {
			boards = append(boards, board)
		}
	}

	// make an array of board ids so we can query
	boardids := make([]string, len(boards))
	for i, board := range boards {
		boardids[i] = board.Id
	}

	// from all possible boards, which ones are enabled
	// even if they are enabled by a different trello user
	var enabledboards []Board
	err = pg.Select(&enabledboards, `
SELECT * FROM boards
WHERE id = ANY (string_to_array($1, ','))
    `, strings.Join(boardids, ","))
	if err != nil && err != sql.ErrNoRows {
		http.Error(w, "failed to fetch enabled boards: "+err.Error(), 500)
		return
	}

	// merge enabled properties on full boards list
	for i, iboard := range boards {
		for _, jboard := range enabledboards {
			if iboard.Id == jboard.Id {
				boards[i].Email = jboard.Email
				boards[i].Enabled = true
			}
		}
	}

	err = tmpl.ExecuteTemplate(w, "account.html", struct {
		Username string
		Email    string
		Boards   []Board
	}{username.(string), email.(string), boards})
	if err != nil {
		log.Warn().Err(err).Msg("failed to render /account")
	}
}

func handleSetupBoard(w http.ResponseWriter, r *http.Request) {
	sess, _ := store.Get(r, "auth-session")
	email, ok1 := sess.Values["email"]
	token, ok2 := sess.Values["token"]
	id, ok3 := sess.Values["id"]
	if !ok1 || !ok2 || !ok3 {
		http.Redirect(w, r, "/auth", http.StatusFound)
		return
	}
	board := r.FormValue("board")
	enabled := r.FormValue("enabled") != "false"

	err := setupBoard(board, id.(string), email.(string), token.(string), enabled)
	if err != nil {
		http.Error(w, "failed to set permissions on board: "+err.Error(), 500)
		return
	}

	http.Redirect(w, r, "/account", http.StatusFound)
}

func returnOk(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
}
