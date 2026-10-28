package sadp

import (
	"bytes"
	"crypto/aes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/TerminalAddict/SADPTools/internal/xmlsafe"
)

type SecurityQuestion struct {
	ID         int    `json:"id"`
	Text       string `json:"text"`
	Configured bool   `json:"configured"`
}
type SecurityAnswer struct {
	ID     int    `json:"id"`
	Answer string `json:"answer"`
}

func (a SecurityAnswer) String() string       { return "security answer (omitted)" }
func (a SecurityAnswer) GoString() string     { return a.String() }
func (a SecurityAnswer) LogValue() slog.Value { return slog.StringValue(a.String()) }

// The original fifteen-question SADP profile. Meanings were checked against
// device-supplied English catalogs; these are independently phrased labels.
// Require the complete advertised ID set. Unknown/custom profiles fail closed.
// This catalog is separate from ISAPI recovery's firmware-supplied catalog.
var sadpQuestionLabels = [...]string{
	"", "What is your father's name?", "What is your mother's name?",
	"What is your senior high school head teacher's name?", "What is your junior high school head teacher's name?",
	"What is the name of the roommate you know best?", "Who has influenced you most?",
	"Who is your favourite celebrity?", "What is your favourite car brand?", "What is your favourite game?",
	"What is your favourite book?", "What is your favourite colour?", "What is your favourite movie?",
	"What is your favourite flower?", "What was the brand of your first mobile phone?", "Where did you fly on your first flight?",
}

func ValidateSecurityAnswers(answers []SecurityAnswer) error {
	if len(answers) != 3 {
		return errors.New("select three distinct security questions and provide all three answers")
	}
	seen := map[int]bool{}
	for _, a := range answers {
		if a.ID < 1 || a.ID >= len(sadpQuestionLabels) || seen[a.ID] {
			return errors.New("select three distinct questions from the supported SADP list")
		}
		seen[a.ID] = true
		if len(a.Answer) == 0 || len(a.Answer) > 128 || !utf8.ValidString(a.Answer) {
			return errors.New("each security answer must contain 1–128 UTF-8 bytes")
		}
		for _, r := range a.Answer {
			if unicode.IsControl(r) || r == 0xfffe || r == 0xffff {
				return errors.New("security answers must not contain control characters")
			}
		}
	}
	return nil
}

// Strict nested shape for getsecurityquestion only. The ordinary command parser
// intentionally continues to reject nested fields on all other reply types.
func ParseSecurityQuestions(data []byte) (CommandReply, []SecurityQuestion, error) {
	bad := errors.New("unsupported or ambiguous SADP security-question reply")
	if len(data) == 0 || len(data) > MaxPacketSize {
		return CommandReply{}, nil, bad
	}
	data = bytes.TrimPrefix(bytes.TrimSpace(bytes.TrimRight(data, "\x00")), []byte{0xef, 0xbb, 0xbf})
	root, err := xmlsafe.Parse(data)
	if err != nil || root.Name != "ProbeMatch" || root.Space != "" || len(root.Attrs) != 0 || strings.TrimSpace(root.Text) != "" {
		return CommandReply{}, nil, bad
	}
	fields := map[string]string{}
	var list *xmlsafe.Node
	for _, n := range root.Children {
		if n.Space != "" || len(n.Attrs) != 0 {
			return CommandReply{}, nil, bad
		}
		if _, exists := fields[n.Name]; exists {
			return CommandReply{}, nil, bad
		}
		fields[n.Name] = strings.TrimSpace(n.Text)
		if n.Name == "QuestionList" {
			list = n
			if fields[n.Name] != "" {
				return CommandReply{}, nil, bad
			}
			continue
		}
		if len(n.Children) != 0 {
			return CommandReply{}, nil, bad
		}
	}
	reply := CommandReply{UUID: fields["Uuid"], MAC: fields["MAC"], Types: fields["Types"], Result: fields["Result"], ErrorCode: fields["ErrorCode"]}
	if !strings.EqualFold(reply.Types, string(GetSecurityQuestions)) {
		return CommandReply{}, nil, bad
	}
	// A negative status can legitimately omit QuestionList; expose no device text.
	if !strings.EqualFold(reply.Result, "success") {
		return reply, nil, nil
	}
	if list == nil || len(list.Children) != 15 {
		return reply, nil, bad
	}
	questions := make([]SecurityQuestion, 0, 15)
	seen := map[int]bool{}
	marked := 0
	for _, q := range list.Children {
		if q.Name != "Question" || q.Space != "" || len(q.Attrs) != 0 || strings.TrimSpace(q.Text) != "" || len(q.Children) != 2 {
			return reply, nil, bad
		}
		values := map[string]string{}
		for _, v := range q.Children {
			if v.Space != "" || len(v.Attrs) != 0 || len(v.Children) != 0 || (v.Name != "ID" && v.Name != "mark") {
				return reply, nil, bad
			}
			if _, exists := values[v.Name]; exists {
				return reply, nil, bad
			}
			values[v.Name] = strings.TrimSpace(v.Text)
		}
		id, e := strconv.Atoi(values["ID"])
		flag := optionalBool(values["mark"])
		if e != nil || id < 1 || id >= len(sadpQuestionLabels) || seen[id] || flag == nil {
			return reply, nil, bad
		}
		seen[id] = true
		if *flag {
			marked++
		}
		questions = append(questions, SecurityQuestion{ID: id, Text: sadpQuestionLabels[id], Configured: *flag})
	}
	if marked != 0 && marked != 3 {
		return reply, nil, bad
	}
	sort.Slice(questions, func(i, j int) bool { return questions[i].ID < questions[j].ID })
	return reply, questions, nil
}

// Binds a reviewed question list to the camera identity/adapter. Contains no
// answers or credentials. A fresh check before writing must match this revision.
func QuestionRevision(identity string, questions []SecurityQuestion) string {
	h := sha256.New()
	_, _ = fmt.Fprint(h, identity)
	for _, q := range questions {
		_, _ = fmt.Fprintf(h, "/%d:%t", q.ID, q.Configured)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func encryptSecurityAnswer(answer string, challenge []byte) (string, error) {
	if len(challenge) != 32 || len(answer) == 0 || len(answer) > 128 {
		return "", errors.New("unsupported security-answer encryption input")
	}
	block, err := aes.NewCipher(challenge[:16])
	if err != nil {
		return "", errors.New("could not prepare security-answer encryption")
	}
	plain := make([]byte, ((len(answer)+15)/16)*16)
	defer clear(plain)
	copy(plain, answer)
	for i := 0; i < len(plain); i += aes.BlockSize {
		block.Encrypt(plain[i:i+16], plain[i:i+16])
	}
	return base64.StdEncoding.EncodeToString(plain), nil
}

// Setup uses the same RSA challenge/password profile as activation. Answers use
// raw UTF-8, zero padding and AES-ECB with the first 16 ASCII challenge bytes;
// they do not use ISAPI recovery's base64/hex encoding or challenge prefix.
func (e *ActivationExchange) SecurityQuestionsCommand(id, mac, code, password string, answers []SecurityAnswer) ([]byte, error) {
	if id == "" || activationMAC(mac) == "" {
		return nil, errors.New("invalid security-question identity")
	}
	if err := ValidateSecurityAnswers(answers); err != nil {
		return nil, err
	}
	challenge, err := e.decryptChallenge(code)
	if err != nil {
		return nil, err
	}
	defer clear(challenge)
	passwordCipher, err := encryptSADPPassword(challenge, password)
	if err != nil {
		return nil, err
	}
	type question struct {
		ID     int    `xml:"ID"`
		Answer string `xml:"Answer"`
		Mark   bool   `xml:"mark"`
	}
	body := struct {
		XMLName   xml.Name   `xml:"Probe"`
		UUID      string     `xml:"Uuid"`
		MAC       string     `xml:"MAC"`
		Types     string     `xml:"Types"`
		Questions []question `xml:"QuestionList>Question"`
		Password  string     `xml:"Password"`
	}{UUID: id, MAC: activationMAC(mac), Types: string(SetSecurityQuestions), Password: passwordCipher}
	for _, a := range answers {
		v, err := encryptSecurityAnswer(a.Answer, challenge)
		if err != nil {
			return nil, err
		}
		body.Questions = append(body.Questions, question{a.ID, v, true})
	}
	b, err := xml.Marshal(body)
	if err != nil {
		return nil, errors.New("could not encode security-question setup")
	}
	return append([]byte(xml.Header), b...), nil
}
