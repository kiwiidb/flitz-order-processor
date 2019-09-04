package storageapi

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/schema"
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
	if err != nil {
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
	decoder := schema.NewDecoder()
	order := Order{}
	whrb := WebHookRequestBody{}
	err = decoder.Decode(order, r.URL.Query())
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
	err = json.Unmarshal(bbytes, &whrb)
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
	//TODO
	//1. check Hash from ON
	//2. add order to tdb, use order id as collection id
	_, err = tdb.CreateNewBatchOfTokens(whrb.ID, order.Amt, order.Value)
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
	//3. add order to flitz vouchers storage
	//4. send e-mail to client
	w.WriteHeader(http.StatusOK)
	return
}
