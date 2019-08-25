package storageapi

import (
	"net/http"

	"github.com/sirupsen/logrus"
)

//WebhookHandler to be called by Opennode
func WebhookHandler(w http.ResponseWriter, r *http.Request) {
	logrus.Infof("%v", r)
	w.WriteHeader(http.StatusOK)
	return
}
