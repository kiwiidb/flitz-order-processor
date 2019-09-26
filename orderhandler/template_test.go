package orderhandler

import (
	"testing"

	"github.com/sirupsen/logrus"
)

func TestTemplating(t *testing.T) {
	testorder := Order{
		Value:    10,
		Amt:      1,
		Currency: "EUR",
		Email:    "test@test.com",
	}
	result, err := createEmailBody(testorder, []string{"testcode"})
	if err != nil {
		t.Fatal(err)
	}
	logrus.Info(result)
}
