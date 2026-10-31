package premise

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"strconv"
	"strings"
)

var (
	errNotJSON  = errors.New("body is not json")
	errPointer  = errors.New("missing pointer")
	errRedirect = errors.New("redirect")
)

func expectedOf(spec Spec) any {
	if spec.HasEquals {
		return spec.Equals
	}
	return spec.Semver
}

func hashBytes(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func judge(spec Spec, body []byte) Result {
	res := Result{Result: ResultError, Expected: expectedOf(spec)}
	raw, err := pointerValue(body, spec.Pointer)
	if err != nil {
		if errors.Is(err, errNotJSON) {
			res.Reason = ReasonNotJSON
			return res
		}
		res.Reason = ReasonPointer
		return res
	}
	if len(raw) > ObservedLimit {
		res.Reason = ReasonObserved
		return res
	}
	res.Observed = append([]byte(nil), raw...)
	if spec.HasEquals {
		return judgeEquals(res, spec.Equals)
	}
	return judgeSemver(res, spec.Semver)
}

func judgeEquals(res Result, expected any) Result {
	observed, err := decodeValue(res.Observed)
	if err != nil {
		res.Reason = ReasonNotJSON
		res.Observed = nil
		return res
	}
	equal, typeOK := equalJSON(expected, observed)
	if !typeOK {
		res.Reason = ReasonType
		return res
	}
	if equal {
		res.Result = ResultPass
		return res
	}
	res.Result = ResultFail
	return res
}

func judgeSemver(res Result, constraint string) Result {
	observed, err := decodeValue(res.Observed)
	if err != nil {
		res.Reason = ReasonNotJSON
		res.Observed = nil
		return res
	}
	text, ok := observed.(string)
	if !ok {
		res.Reason = ReasonType
		return res
	}
	got, ok := parseVersion(text)
	if !ok {
		res.Reason = ReasonSemver
		return res
	}
	want, op, ok := parseConstraint(constraint)
	if !ok {
		res.Reason = ReasonSemver
		return res
	}
	if holds(op, compareVersion(got, want)) {
		res.Result = ResultPass
		return res
	}
	res.Result = ResultFail
	return res
}

func decodeValue(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func pointerValue(body []byte, pointer string) ([]byte, error) {
	if !json.Valid(body) {
		return nil, errNotJSON
	}
	compact := new(bytes.Buffer)
	if err := json.Compact(compact, body); err != nil {
		return nil, errNotJSON
	}
	cur := compact.Bytes()
	if pointer == "" {
		return cur, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, errPointer
	}
	for _, part := range strings.Split(pointer[1:], "/") {
		token, ok := unescapeToken(part)
		if !ok {
			return nil, errPointer
		}
		next, err := stepPointer(cur, token)
		if err != nil {
			return nil, err
		}
		cur = next
	}
	out := new(bytes.Buffer)
	if err := json.Compact(out, cur); err != nil {
		return nil, errNotJSON
	}
	return out.Bytes(), nil
}

func unescapeToken(token string) (string, bool) {
	if !strings.Contains(token, "~") {
		return token, true
	}
	var b strings.Builder
	for i := 0; i < len(token); i++ {
		if token[i] != '~' {
			b.WriteByte(token[i])
			continue
		}
		if i+1 >= len(token) {
			return "", false
		}
		switch token[i+1] {
		case '0':
			b.WriteByte('~')
		case '1':
			b.WriteByte('/')
		default:
			return "", false
		}
		i++
	}
	return b.String(), true
}

func stepPointer(cur []byte, token string) ([]byte, error) {
	if len(cur) == 0 {
		return nil, errPointer
	}
	switch cur[0] {
	case '{':
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(cur, &obj); err != nil {
			return nil, errNotJSON
		}
		next, ok := obj[token]
		if !ok {
			return nil, errPointer
		}
		return next, nil
	case '[':
		var arr []json.RawMessage
		if err := json.Unmarshal(cur, &arr); err != nil {
			return nil, errNotJSON
		}
		if !arrayIndex(token) {
			return nil, errPointer
		}
		index, err := strconv.Atoi(token)
		if err != nil || index < 0 || index >= len(arr) {
			return nil, errPointer
		}
		return arr[index], nil
	default:
		return nil, errPointer
	}
}

func arrayIndex(token string) bool {
	if token == "0" {
		return true
	}
	if token == "" || token[0] < '1' || token[0] > '9' {
		return false
	}
	for _, r := range token[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func equalJSON(expected, observed any) (equal bool, typeOK bool) {
	if jsonKind(expected) != jsonKind(observed) {
		return false, false
	}
	switch jsonKind(expected) {
	case "null":
		return true, true
	case "bool":
		return expected.(bool) == observed.(bool), true
	case "string":
		return expected.(string) == observed.(string), true
	case "number":
		return numbersEqual(expected, observed), true
	case "array":
		left, _ := expected.([]any)
		right, _ := observed.([]any)
		if len(left) != len(right) {
			return false, true
		}
		for i := range left {
			same, ok := equalJSON(left[i], right[i])
			if !ok {
				return false, false
			}
			if !same {
				return false, true
			}
		}
		return true, true
	case "object":
		left, _ := expected.(map[string]any)
		right, _ := observed.(map[string]any)
		if len(left) != len(right) {
			return false, true
		}
		for key, value := range left {
			other, ok := right[key]
			if !ok {
				return false, true
			}
			same, typeOK := equalJSON(value, other)
			if !typeOK {
				return false, false
			}
			if !same {
				return false, true
			}
		}
		return true, true
	default:
		return false, false
	}
}

func jsonKind(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case string:
		return "string"
	case float64, json.Number:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "other"
	}
}

func numbersEqual(left, right any) bool {
	a, aok := asRat(left)
	b, bok := asRat(right)
	if !aok || !bok {
		return false
	}
	return a.Cmp(b) == 0
}

func asRat(value any) (*big.Rat, bool) {
	ratio := new(big.Rat)
	switch typed := value.(type) {
	case json.Number:
		if _, ok := ratio.SetString(typed.String()); !ok {
			return nil, false
		}
		return ratio, true
	case float64:
		text := strconv.FormatFloat(typed, 'f', -1, 64)
		if _, ok := ratio.SetString(text); !ok {
			return nil, false
		}
		return ratio, true
	default:
		return nil, false
	}
}

type semver struct {
	major string
	minor string
	patch string
	pre   []string
}

func parseConstraint(text string) (semver, string, bool) {
	op := ""
	rest := text
	switch {
	case strings.HasPrefix(text, ">="):
		op, rest = ">=", text[2:]
	case strings.HasPrefix(text, "<="):
		op, rest = "<=", text[2:]
	case strings.HasPrefix(text, "="):
		op, rest = "=", text[1:]
	case strings.HasPrefix(text, ">"):
		op, rest = ">", text[1:]
	case strings.HasPrefix(text, "<"):
		op, rest = "<", text[1:]
	default:
		return semver{}, "", false
	}
	parsed, ok := parseVersion(rest)
	if !ok {
		return semver{}, "", false
	}
	return parsed, op, true
}

func parseVersion(text string) (semver, bool) {
	if strings.Contains(text, "+") || strings.Contains(text, " ") {
		return semver{}, false
	}
	main, pre, hasPre := strings.Cut(text, "-")
	parts := strings.Split(main, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	for _, part := range parts {
		if !numericIdent(part) {
			return semver{}, false
		}
	}
	parsed := semver{major: parts[0], minor: parts[1], patch: parts[2]}
	if !hasPre {
		return parsed, true
	}
	if pre == "" {
		return semver{}, false
	}
	idents := strings.Split(pre, ".")
	for _, ident := range idents {
		if !preIdent(ident) {
			return semver{}, false
		}
	}
	parsed.pre = idents
	return parsed, true
}

func numericIdent(text string) bool {
	if text == "0" {
		return true
	}
	if text == "" || text[0] < '1' || text[0] > '9' {
		return false
	}
	for _, r := range text[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func preIdent(text string) bool {
	if numericIdent(text) {
		return true
	}
	if text == "" {
		return false
	}
	letter := false
	for _, r := range text {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r == '-':
			letter = true
		default:
			return false
		}
	}
	return letter
}

func compareVersion(got, want semver) int {
	if c := compareNumber(got.major, want.major); c != 0 {
		return c
	}
	if c := compareNumber(got.minor, want.minor); c != 0 {
		return c
	}
	if c := compareNumber(got.patch, want.patch); c != 0 {
		return c
	}
	if len(got.pre) == 0 && len(want.pre) == 0 {
		return 0
	}
	if len(got.pre) == 0 {
		return 1
	}
	if len(want.pre) == 0 {
		return -1
	}
	n := len(got.pre)
	if len(want.pre) < n {
		n = len(want.pre)
	}
	for i := 0; i < n; i++ {
		if c := compareIdent(got.pre[i], want.pre[i]); c != 0 {
			return c
		}
	}
	return compareLen(len(got.pre), len(want.pre))
}

func compareNumber(left, right string) int {
	if len(left) != len(right) {
		return compareLen(len(left), len(right))
	}
	return strings.Compare(left, right)
}

func compareIdent(left, right string) int {
	leftNum, rightNum := numericIdent(left), numericIdent(right)
	if leftNum && rightNum {
		return compareNumber(left, right)
	}
	if leftNum && !rightNum {
		return -1
	}
	if !leftNum && rightNum {
		return 1
	}
	return strings.Compare(left, right)
}

func compareLen(left, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func holds(op string, cmp int) bool {
	switch op {
	case "=":
		return cmp == 0
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	default:
		return false
	}
}
