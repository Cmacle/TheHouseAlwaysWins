// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// Settings by name, for the server window and the "set"/"get" commands.
// Keys are the names in server.json; nested ones are dotted
// ("discord.enabled").

// Setting is one server.json value.
type Setting struct {
	Key   string
	Kind  string // bool, int, float, string, list
	Value string
	Group string // "" or the nested section (discord, http)
}

// secretKeys are shown masked.
var secretKeys = map[string]bool{"discord.bot_token": true, "discord.webhook_url": true}

// SettingsOf lists every setting in c.
func SettingsOf(c *Config) []Setting { return settingsOfValue(reflect.ValueOf(c).Elem(), "", "") }

// settingsOfValue lists the json fields of a struct as settings.
func settingsOfValue(v reflect.Value, prefix, group string) []Setting {
	var out []Setting
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := strings.Split(f.Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" || !f.IsExported() {
			continue
		}
		fv := v.Field(i)
		key := prefix + tag
		s := Setting{Key: key, Group: group}
		switch fv.Kind() {
		case reflect.Struct:
			out = append(out, settingsOfValue(fv, key+".", tag)...)
			continue
		case reflect.Bool:
			s.Kind, s.Value = "bool", strconv.FormatBool(fv.Bool())
		case reflect.Int, reflect.Int64, reflect.Int32:
			s.Kind, s.Value = "int", strconv.FormatInt(fv.Int(), 10)
		case reflect.Float64, reflect.Float32:
			s.Kind, s.Value = "float", strconv.FormatFloat(fv.Float(), 'f', -1, 64)
		case reflect.String:
			s.Kind, s.Value = "string", fv.String()
		case reflect.Slice:
			var parts []string
			for j := 0; j < fv.Len(); j++ {
				parts = append(parts, fmt.Sprint(fv.Index(j).Interface()))
			}
			s.Kind, s.Value = "list", strings.Join(parts, ",")
		default:
			continue
		}
		out = append(out, s)
	}
	return out
}

// parseSetting reads a typed value for a setting.
func parseSetting(key, kind, value string) (any, error) {
	value = strings.TrimSpace(value)
	switch kind {
	case "bool":
		switch strings.ToLower(value) {
		case "true", "on", "yes", "1":
			return true, nil
		case "false", "off", "no", "0":
			return false, nil
		}
		return nil, fmt.Errorf("%s is on or off", key)
	case "int":
		n, err := strconv.ParseInt(strings.ReplaceAll(value, ",", ""), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s needs a whole number", key)
		}
		return n, nil
	case "float":
		n, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("%s needs a number", key)
		}
		return n, nil
	case "list":
		parts := []string{}
		for _, p := range strings.Split(value, ",") {
			if p = strings.TrimSpace(p); p != "" {
				parts = append(parts, p)
			}
		}
		return parts, nil
	}
	return value, nil
}

// SettingInfos lists the settings for the protocol; secrets are blanked
// when masked is set.
func (s *Server) SettingInfos(masked bool) []proto.SettingInfo {
	var out []proto.SettingInfo
	for _, st := range SettingsOf(s.cfg()) {
		v := st.Value
		if masked && secretKeys[st.Key] && v != "" {
			v = "(set)"
		}
		out = append(out, proto.SettingInfo{Key: st.Key, Label: LabelOf(st.Key), Kind: st.Kind, Value: v, Group: st.Group})
	}
	return out
}

// LabelOf turns "card_reveal_min_seconds" into "Card reveal min seconds".
func LabelOf(key string) string {
	if i := strings.LastIndex(key, "."); i >= 0 {
		key = key[i+1:]
	}
	s := strings.ReplaceAll(key, "_", " ")
	for _, w := range [][2]string{{"ms", "(ms)"}, {"motd", "message of the day"}, {"http", "HTTP"}, {"id", "ID"}, {"url", "URL"}} {
		s = replaceWord(s, w[0], w[1])
	}
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func replaceWord(s, old, new string) string {
	f := strings.Fields(s)
	for i := range f {
		if f[i] == old {
			f[i] = new
		}
	}
	return strings.Join(f, " ")
}

// SetSetting changes one setting in server.json and reloads it. The value is
// checked first: a bad value changes nothing.
func (s *Server) SetSetting(key, value string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	var kind string
	for _, st := range SettingsOf(s.cfg()) {
		if st.Key == key {
			kind = st.Kind
		}
	}
	if kind == "" {
		return fmt.Errorf("there's no setting called %q", key)
	}
	value = strings.TrimSpace(value)
	if secretKeys[key] && value == "(set)" {
		return nil // the masked placeholder came back unchanged
	}
	v, err := parseSetting(key, kind, value)
	if err != nil {
		return err
	}
	if s.configPath == "" {
		return errors.New("this server has no config file")
	}
	b, err := os.ReadFile(s.configPath)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("server.json doesn't parse: %v", err)
	}
	m := raw
	parts := strings.Split(key, ".")
	for _, p := range parts[:len(parts)-1] {
		sub, ok := m[p].(map[string]any)
		if !ok {
			sub = map[string]any{}
			m[p] = sub
		}
		m = sub
	}
	m[parts[len(parts)-1]] = v
	merged, _ := json.Marshal(raw)
	// check it before writing it
	test := DefaultConfig()
	if err := json.Unmarshal(merged, test); err != nil {
		return err
	}
	if err := test.validate(); err != nil {
		return err
	}
	out, _ := json.MarshalIndent(test, "", "  ")
	if err := writeFileSafe(s.configPath, append(out, '\n'), true); err != nil {
		return err
	}
	return s.reload()
}
