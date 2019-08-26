package storageapi

import (
	"io/ioutil"
	"net/http"

	"github.com/sirupsen/logrus"
)

//Order some vouchers
type Order struct {
	Value    int
	Amt      int
	Currency string
	Email    string
}

//WebHookRequestBody to check authenticity of OpenNode request
type WebHookRequestBody struct {
	HashedOrder string
	ID          string
}

//WebhookHandler to be called by Opennode
func WebhookHandler(w http.ResponseWriter, r *http.Request) {
	logrus.Infof("%v", r)
	bbytes, err := ioutil.ReadAll(r.Body)
	logrus.Info(r.URL.Query())
	if err != nil {
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
	}
	logrus.Info(string(bbytes))
	w.WriteHeader(http.StatusOK)
	//TODO
	//1. check Hash from ON
	//2. add order to tdb
	//3. add order to flitz vouchers storage
	//4. send e-mail to client
	return
}
