package storageapi

import (
	"io/ioutil"
	"net/http"

	"github.com/sirupsen/logrus"
)

//WebhookHandler to be called by Opennode
func WebhookHandler(w http.ResponseWriter, r *http.Request) {
	logrus.Infof("%v", r)
	bbytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
	}
	logrus.Info(string(bbytes))
	w.WriteHeader(http.StatusOK)
	return
}
