package alteon

import (
	"encoding/json"
	"strconv"

	radwaregosdk "github.com/Radware/radware_go_sdk"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
)

// Diese Datei buendelt die wiederkehrende REST-CRUD-Logik, die im urspruenglichen
// Provider in jeder Ressource dupliziert war. Neue Ressourcen (VRRP, AdvHC, PIP,
// Filter, Content/Data Class, AppShape) nutzen diese Helfer, statt den Code zu kopieren.
//
// Konventionen der Alteon-REST-API (verifiziert an der RDWRAlteonRestDoc, FW 34.0.9+):
//   - Anlegen:  POST   /config/<Table>/<Key>     (client.CreateItem)
//   - Aendern:  PUT    /config/<Table>/<Key>     (client.UpdateItem)
//   - Loeschen: DELETE /config/<Table>/<Key>     (client.DeleteItem) -- von Radware empfohlen
//   - Lesen:    GET    /config/<Table>/<Key>     (client.GetItem)
//
// GET liefert {"<Table>":[ { feld: wert, ... } ]} zurueck.

// configPath baut den REST-Pfad fuer einen Tabelleneintrag.
func configPath(table, key string) string {
	return "/config/" + table + "/" + key + "/"
}

// writeItem fuehrt CreateItem (POST) bzw. UpdateItem (PUT) aus und wertet die
// Alteon-typische Antwort einheitlich aus: HTTP-Status 200 UND im JSON-Body
// "status":"ok". Bei isCreate wird POST verwendet, sonst PUT.
func writeItem(client *radwaregosdk.New_Client, api string, payload map[string]interface{}, isCreate bool) diag.Diagnostics {
	var diags diag.Diagnostics

	body, err := json.MarshalIndent(payload, "", "    ")
	if err != nil {
		return append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Error encoding JSON: " + err.Error(),
			Detail:   err.Error(),
		})
	}

	var status int
	var message string
	verb := "UpdateItem"
	if isCreate {
		status, message, err = client.CreateItem(api, body, nil)
		verb = "CreateItem"
	} else {
		status, message, err = client.UpdateItem(api, body, nil)
	}

	return checkWriteResponse(diags, verb, api, status, message, err)
}

// checkWriteResponse prueft die Antwort einer POST/PUT-Operation.
func checkWriteResponse(diags diag.Diagnostics, verb, api string, status int, message string, err error) diag.Diagnostics {
	detail := "Status Code Received: " + strconv.Itoa(status) + "\nResponse Received:\n" + message + "\nAPI Call Made is: " + api

	if err != nil {
		return append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "REST " + verb + " failed with error: " + err.Error(),
			Detail:   detail,
		})
	}
	if status != 200 {
		return append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "REST " + verb + " failed, response code: " + strconv.Itoa(status),
			Detail:   detail,
		})
	}

	respBody := map[string]interface{}{}
	json.Unmarshal([]byte(message), &respBody)
	if s, ok := respBody["status"].(string); ok && s != "ok" {
		return append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "REST " + verb + " failed, status in 200 response: " + s,
			Detail:   detail,
		})
	}

	return diags
}

// deleteItem fuehrt DELETE auf einem Tabelleneintrag aus.
func deleteItem(client *radwaregosdk.New_Client, api string) diag.Diagnostics {
	var diags diag.Diagnostics
	status, message, err := client.DeleteItem(api, nil, nil)
	return checkWriteResponse(diags, "DeleteItem", api, status, message, err)
}

// readItem liest einen einzelnen Tabelleneintrag per GET und gibt die Felder
// als Map zurueck. Liefert (nil, false) zurueck, wenn der Eintrag nicht (mehr)
// existiert -- der Aufrufer setzt dann d.SetId("") (Drift: Objekt geloescht).
// Bei einem echten Fehler werden diags gesetzt und (nil, false) zurueckgegeben.
func readItem(client *radwaregosdk.New_Client, api, table string) (map[string]interface{}, bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	status, message, err := client.GetItem(api, nil, nil)
	detail := "Status Code Received: " + strconv.Itoa(status) + "\nResponse Received:\n" + message + "\nAPI Call Made is: " + api

	if err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "REST GetItem failed with error: " + err.Error(),
			Detail:   detail,
		})
		return nil, false, diags
	}

	// 404 / nicht gefunden -> Objekt existiert nicht mehr, kein Fehler.
	if status == 404 {
		return nil, false, diags
	}
	if status != 200 {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "REST GetItem failed, response code: " + strconv.Itoa(status),
			Detail:   detail,
		})
		return nil, false, diags
	}

	parsed := map[string]interface{}{}
	json.Unmarshal([]byte(message), &parsed)

	// Body mit "status":"err" (z.B. "object not found") -> existiert nicht mehr.
	if s, ok := parsed["status"].(string); ok && s == "err" {
		return nil, false, diags
	}

	rows, ok := parsed[table].([]interface{})
	if !ok || len(rows) == 0 {
		// Leere Antwort: Eintrag existiert nicht (mehr).
		return nil, false, diags
	}

	item, ok := rows[0].(map[string]interface{})
	if !ok {
		return nil, false, diags
	}
	return item, true, diags
}

// --- Enum/Bool-Mapping -------------------------------------------------------
// Viele Alteon-Felder sind 1/2-Enums (1=enabled, 2=disabled). Im HCL bilden wir
// sie auf bool ab, damit Konfiguration lesbar bleibt.

// boolToEnable wandelt bool -> 1 (enabled) / 2 (disabled).
func boolToEnable(b bool) int {
	if b {
		return 1
	}
	return 2
}

// enableToBool wandelt einen aus JSON gelesenen Wert (1/2, ggf. als float64 oder
// string) in bool. 1 => true, alles andere => false.
func enableToBool(v interface{}) bool {
	switch t := v.(type) {
	case float64:
		return int(t) == 1
	case int:
		return t == 1
	case string:
		return t == "1"
	}
	return false
}

// asInt liest einen JSON-Zahlenwert robust als int (JSON liefert float64).
func asInt(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		if n, err := strconv.Atoi(t); err == nil {
			return n
		}
	}
	return 0
}

// asString liest einen JSON-Wert robust als string.
func asString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.Itoa(int(t))
	}
	return ""
}

// --- Bitmap-Dekodierung (PIP PortMap/VlanMap, Filter FiltBmap, ...) -----------
// Alteon liefert solche Mengen als doppelpunkt-getrennte Hex-Bytes, z.B.
// "00:00:00:20:00:...". Konvention: Byte 0 / Bit 7 (MSB) entspricht Element 1,
// die Nummerierung laeuft Byte fuer Byte, innerhalb eines Bytes MSB->LSB.
//
// HINWEIS: Die exakte Element-Nummer (Port-/VLAN-Nummer) sollte am Geraet gegen
// ein bekanntes Beispiel verifiziert werden. Format (Hex-Byte-Bitmap) ist sicher;
// die 1-basierte MSB-first-Nummerierung ist die uebliche Alteon-Konvention.

// decodeHexBitmap wandelt "00:20:..." in die sortierte Liste gesetzter
// 1-basierter Element-Nummern.
func decodeHexBitmap(s string) []int {
	var result []int
	if s == "" {
		return result
	}
	bytes := splitColon(s)
	for byteIdx, hb := range bytes {
		val := hexByte(hb)
		for bit := 0; bit < 8; bit++ {
			// MSB (bit 7) = niedrigste Elementnummer im Byte.
			if val&(1<<(7-bit)) != 0 {
				elem := byteIdx*8 + bit + 1 // 1-basiert
				result = append(result, elem)
			}
		}
	}
	return result
}

func splitColon(s string) []string {
	var out []string
	cur := ""
	for _, c := range s {
		if c == ':' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(c)
		}
	}
	out = append(out, cur)
	return out
}

func hexByte(h string) int {
	n := 0
	for _, c := range h {
		n *= 16
		switch {
		case c >= '0' && c <= '9':
			n += int(c - '0')
		case c >= 'a' && c <= 'f':
			n += int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			n += int(c-'A') + 10
		}
	}
	return n
}

// intSliceToSet / setDelta helfen bei der deklarativen Delta-Berechnung von Mengen.
// Liefert Elemente, die in want, aber nicht in have sind (=hinzuzufuegen), und
// Elemente in have, aber nicht in want (=zu entfernen).
func setDelta(want, have []int) (toAdd, toRemove []int) {
	wantSet := map[int]bool{}
	haveSet := map[int]bool{}
	for _, v := range want {
		wantSet[v] = true
	}
	for _, v := range have {
		haveSet[v] = true
	}
	for v := range wantSet {
		if !haveSet[v] {
			toAdd = append(toAdd, v)
		}
	}
	for v := range haveSet {
		if !wantSet[v] {
			toRemove = append(toRemove, v)
		}
	}
	return toAdd, toRemove
}

// interfaceListToInts konvertiert ein TypeList/TypeSet von ints (aus dem Schema)
// in []int.
func interfaceListToInts(raw []interface{}) []int {
	out := make([]int, 0, len(raw))
	for _, v := range raw {
		out = append(out, v.(int))
	}
	return out
}

// intToStr ist eine Kurzform fuer strconv.Itoa, lokal genutzt fuer Eintrags-Keys.
func intToStr(n int) string {
	return strconv.Itoa(n)
}

// readTable liest eine GANZE Tabelle (ohne Key) und gibt alle Zeilen als Slice
// von Maps zurueck. Genutzt fuer Tabellen, die clientseitig nach einer Spalte
// gefiltert werden muessen (z.B. Gruppen-Mitglieder pro Gruppe).
// found=false bei 404/leer.
func readTable(client *radwaregosdk.New_Client, table string) ([]map[string]interface{}, bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	api := "/config/" + table
	status, message, err := client.GetItem(api, nil, nil)
	if err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "REST GetItem (table) failed: " + err.Error(),
			Detail:   "API: " + api,
		})
		return nil, false, diags
	}
	if status == 404 {
		return nil, false, diags
	}
	if status != 200 {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "REST GetItem (table) failed, code: " + strconv.Itoa(status),
			Detail:   "API: " + api + "\n" + message,
		})
		return nil, false, diags
	}
	parsed := map[string]interface{}{}
	json.Unmarshal([]byte(message), &parsed)
	rows, ok := parsed[table].([]interface{})
	if !ok || len(rows) == 0 {
		return nil, false, diags
	}
	out := make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		if mm, ok := r.(map[string]interface{}); ok {
			out = append(out, mm)
		}
	}
	return out, true, diags
}
