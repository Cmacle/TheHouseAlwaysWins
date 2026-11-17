package server

import (
	"encoding/json"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/cosmetics"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
	"html/template"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var boardTmpl = template.Must(template.New("b").Funcs(template.FuncMap{
	"signed": func(n int64) string {
		if n > 0 {
			return "+" + commas(n)
		}
		return commas(n)
	},
	"commas": commas,
	"inc":    func(i int) int { return i + 1 },
	"title": func(id string) string {
		if it, ok := cosmetics.Get(id); ok {
			return it.Name
		}
		return ""
	},
	"tierColor": func(id string) string {
		if it, ok := cosmetics.Get(id); ok {
			return cosmetics.TierColors[it.Tier]
		}
		return "#ccc"
	},
}).Parse(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="refresh" content="30">
<title>{{.Name}} - Leaderboard</title>
<style>
body{background:#0d2a1c;color:#f3ead2;font-family:"Courier New",monospace;margin:0;padding:24px}
h1{color:#f2c14e;text-align:center;letter-spacing:2px;text-shadow:3px 3px 0 #000}
h2{color:#f2c14e;text-align:center;margin:28px 0 10px}
table{margin:0 auto;border-collapse:collapse;min-width:min(760px,100%);background:#123d28;border:4px solid #3b2414;box-shadow:6px 6px 0 #000}
th,td{padding:7px 12px;text-align:right}th{background:#3b2414;color:#f2c14e}
td.n,th.n{text-align:left}tr:nth-child(even){background:#164a31}td.hl{font-weight:bold;font-size:1.1em}
img{width:32px;height:32px;image-rendering:pixelated;vertical-align:middle;margin-right:8px;border:2px solid #000;background:#2b5}
.pos{color:#7dff8a}.neg{color:#ff6b6b}.on{color:#7dff8a;font-size:12px}.t{font-size:12px;display:block;margin-left:44px;margin-top:-6px}p{text-align:center;color:#9bb}
</style></head><body>
<h1>{{.Name}}</h1>
{{range .Boards}}<h2>{{.Title}}</h2>
<table><tr><th>#</th><th class="n">Player</th><th>Chips now</th><th>Lifetime</th><th>Hands</th><th>Blackjacks</th></tr>
{{$by := .By}}{{range $i, $p := .Entries}}<tr><td>{{if eq $i 0}}{{if eq $by "balance"}}&#128081;{{else}}&#127942;{{end}}{{else}}{{inc $i}}{{end}}</td>
<td class="n"><img src="avatar/{{$p.Username}}.png" alt="">{{$p.Username}}{{if $p.Online}} <span class="on">&#9679;</span>{{end}}<span class="t" style="color:{{tierColor $p.Title}}">{{title $p.Title}}</span></td>
<td class="{{if eq $by "balance"}}hl{{end}}">{{commas $p.Balance}}</td>
<td class="{{if gt $p.Net 0}}pos{{else if lt $p.Net 0}}neg{{end}} {{if eq $by "net"}}hl{{end}}">{{signed $p.Net}}</td><td>{{$p.HandsPlayed}}</td><td>{{$p.Blackjacks}}</td></tr>
{{else}}<tr><td colspan="6" class="n">No players yet.</td></tr>{{end}}
</table>{{end}}
<p>Lifetime = everything won minus everything lost, minus daily refills. Updated {{.Time}} &middot; refreshes every 30s &middot;
<a style="color:#f2c14e" href="leaderboard.json">json</a></p>
</body></html>`))

type board struct {
	Title, By string
	Entries   []proto.Profile
}

func commas(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// StartHTTP serves the web leaderboard if enabled.
func (s *Server) StartHTTP() {
	hc := s.cfg().HTTPLeaderboard
	if !hc.Enabled {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = boardTmpl.Execute(w, map[string]any{
			"Name": s.cfg().ServerName,
			"Boards": []board{
				{"Richest Right Now", SortBalance, s.Store.Leaderboard(50, SortBalance)},
				{"Lifetime Gain / Loss", SortNet, s.Store.Leaderboard(50, SortNet)},
			},
			"Time": time.Now().Format("Jan 2 15:04:05"),
		})
	})
	mux.HandleFunc("/leaderboard.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		by := SortNet
		if r.URL.Query().Get("sort") == SortBalance {
			by = SortBalance
		}
		_ = json.NewEncoder(w).Encode(s.Store.Leaderboard(100, by))
	})
	mux.HandleFunc("/avatar/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/avatar/"), ".png")
		data, _ := s.Store.Avatar(name)
		if data == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "max-age=60")
		_, _ = w.Write(data)
	})
	addr := net.JoinHostPort(hc.Address, strconv.Itoa(hc.Port))
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		s.Log.Printf("web leaderboard on http://%s/", addr)
		if err := srv.ListenAndServe(); err != nil {
			s.Log.Printf("web leaderboard stopped: %v", err)
		}
	}()
}
