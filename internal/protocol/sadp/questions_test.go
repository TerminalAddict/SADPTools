package sadp

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
)

func questionFixture() string {
	var b strings.Builder
	b.WriteString(`<ProbeMatch><Uuid>synthetic-id</Uuid><Types>getsecurityquestion</Types><Result>success</Result><QuestionList>`)
	for i := 1; i <= 15; i++ {
		fmt.Fprintf(&b, "<Question><ID>%d</ID><mark>false</mark></Question>", i)
	}
	b.WriteString("</QuestionList></ProbeMatch>")
	return b.String()
}

func TestSecurityQuestionProfileAndStrictParsing(t *testing.T) {
	reply, qs, err := ParseSecurityQuestions([]byte(questionFixture()))
	if err != nil || reply.UUID != "synthetic-id" || len(qs) != 15 || qs[0].Text != "What is your father's name?" || qs[14].Configured {
		t.Fatal("wrong question profile", err)
	}
	for _, change := range [][2]string{
		{"<ID>1</ID>", "<ID>16</ID>"}, {"<ID>2</ID>", "<ID>1</ID>"},
		{"<mark>false</mark>", "<mark>unknown</mark>"}, {"<mark>false</mark>", "<mark>true</mark>"},
		{"<Question><ID>1</ID><mark>false</mark></Question>", ""},
		{"<Question><ID>1</ID>", "<Question><ID>1</ID><ID>2</ID>"},
		{"<Question><ID>1</ID>", "<Question><ID><ID>1</ID></ID>"},
		{"<ProbeMatch>", "<ProbeMatch xmlns='other'>"},
		{"</ProbeMatch>", "</ProbeMatch><ProbeMatch/>"},
		{"<Uuid>synthetic-id</Uuid>", "<Uuid>synthetic-id</Uuid><Uuid>ambiguous</Uuid>"},
		{"<QuestionList>", "<QuestionList unexpected='attribute'>"},
	} {
		if _, _, err := ParseSecurityQuestions([]byte(strings.Replace(questionFixture(), change[0], change[1], 1))); err == nil {
			t.Fatalf("accepted ambiguous/unknown profile: %s", change[0])
		}
	}
	if _, err := ParseCommandReply([]byte(questionFixture())); err == nil {
		t.Fatal("ordinary parser was weakened to accept nested question data")
	}
}

func TestSecurityAnswerIndependentVectors(t *testing.T) {
	// Synthetic vectors independently compared with the archived SDK encryption
	// routine under emulation. Full blocks do not get an extra padding block.
	for _, tc := range []struct{ answer, want string }{
		{"Synthetic answer", "nnrepC/C7i1ProTHWjNy3g=="},
		{"Synthetic<&> answer", "LYto2ty3zD3kQkN5x0BnzaOduZmh81cy3Xq9g/rHTUE="},
		{"Synthetic 漢字", "5RPYQfq+hRexXgFNi/mQaA=="},
	} {
		got, err := encryptSecurityAnswer(tc.answer, []byte("0123456789abcdef0123456789abcdef"))
		if err != nil || got != tc.want {
			t.Fatal("independent answer vector mismatch", err)
		}
	}
}

func TestSecuritySetupWireAndCredentialIsolation(t *testing.T) {
	e, err := NewActivationExchange()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	code, err := rsa.EncryptPKCS1v15(rand.Reader, &e.key.PublicKey, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	answers := []SecurityAnswer{{1, "Synthetic answer"}, {2, "Synthetic<&> answer"}, {15, "Synthetic 漢字"}}
	payload, err := e.SecurityQuestionsCommand("synthetic-uuid", "02:00:00:00:00:01", base64.StdEncoding.EncodeToString(code), "Synthetic<&>1", answers)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Types, MAC, Password string
		Questions            []struct {
			ID     int
			Answer string
			Mark   bool `xml:"mark"`
		} `xml:"QuestionList>Question"`
	}
	if err = xml.Unmarshal(payload, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Types != "setsecurityquestion" || wire.MAC != "02-00-00-00-00-01" || len(wire.Questions) != 3 || wire.Password != "cnJ+iB7c/QEApxhoeQm1ZQG//N7uff8FO6nRLrIBzNQ=" {
		t.Fatal("wrong setup envelope or password encryption")
	}
	for i, q := range wire.Questions {
		if q.ID != answers[i].ID || !q.Mark || strings.Contains(string(payload), answers[i].Answer) {
			t.Fatal("wrong selected question/unencrypted answer")
		}
	}
	if wire.Questions[0].Answer != "nnrepC/C7i1ProTHWjNy3g==" || strings.Contains(string(payload), "Synthetic") {
		t.Fatal("wrong answer encryption")
	}
	if _, err = e.SecurityQuestionsCommand("id", "02:00:00:00:00:01", "invalid", "Synthetic<&>1", answers); err == nil {
		t.Fatal("bad challenge accepted")
	}
	if _, err = Command("id", "02:00:00:00:00:01", SetSecurityQuestions, "plaintext", "", NetworkSettings{}); err == nil {
		t.Fatal("plaintext bypass")
	}
}

func TestSecurityAnswerValidationAndRedaction(t *testing.T) {
	for _, answers := range [][]SecurityAnswer{
		{{1, "one"}, {1, "two"}, {3, "three"}}, {{0, "one"}, {2, "two"}, {3, "three"}},
		{{1, ""}, {2, "two"}, {3, "three"}}, {{1, strings.Repeat("é", 65)}, {2, "two"}, {3, "three"}},
		{{1, "control\n"}, {2, "two"}, {3, "three"}}, {{1, "one"}},
	} {
		if ValidateSecurityAnswers(answers) == nil {
			t.Fatal("invalid answers accepted")
		}
	}
	answers := []SecurityAnswer{{1, " one "}, {2, "Synthetic 漢字"}, {3, strings.Repeat("é", 64)}}
	if err := ValidateSecurityAnswers(answers); err != nil {
		t.Fatal(err)
	}
	for _, a := range answers {
		for _, s := range []string{fmt.Sprintf("%v", a), fmt.Sprintf("%+v", a), fmt.Sprintf("%#v", a)} {
			if strings.Contains(s, a.Answer) {
				t.Fatal("answer exposed by formatting")
			}
		}
	}
}
