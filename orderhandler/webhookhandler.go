package orderhandler

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/schema"
	"github.com/kiwiidb/bliksem-library/tokendb"
	"github.com/kiwiidb/bliksem-library/utils"
	"github.com/kiwiidb/bliksem-library/vouchertemplating"
	"github.com/koding/multiconfig"
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

var vt *vouchertemplating.VoucherTemplater
var tdb *tokendb.TokenDB

func init() {
	//init firebase
	vt = &vouchertemplating.VoucherTemplater{}
	m := multiconfig.EnvironmentLoader{}
	err := m.Load(vt)
	if err != nil {
		logrus.Fatal(err)
	}
	m.PrintEnvs(vt)
	logrus.Info(vt.FirebaseAdminCredentials)
	vt.InitFirebase()

	//init database
	conf := tokendb.Config{}
	m = multiconfig.EnvironmentLoader{}
	err = m.Load(&conf)
	if err != nil {
		logrus.Fatal(err)
	}
	m.PrintEnvs(conf)
	logrus.Info(conf)
	err = tdb.Initialize(conf)
	if err != nil {
		logrus.Fatal(err)
	}

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
	_, err = tdb.CreateNewBatchOfTokens(whrb.ID, order.Amt, order.Value, order.Currency, true) //online sold vouchers are always already on
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
	codes, err := tdb.GetAllTokensInCollection(whrb.ID)
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
	formattedCodes := []string{}
	formatString := "https://flitz-api-now.kwintendebacker.now.sh/lnurl-primary/%s/%s"
	for _, code := range codes {
		toAppend, err := utils.EncodeToLNURL(fmt.Sprintf(formatString, whrb.ID, code.ID))
		if err != nil {
			logrus.Error(err)
			http.Error(w, "something wrong decoding", http.StatusBadRequest)
			return
		}
		formattedCodes = append(formattedCodes, toAppend)
	}
	var storageURL string
	if order.Amt > 1 {
		storageURL, err = vt.CreateAndUploadZipFromCodes(formattedCodes, whrb.ID)
		if err != nil {
			logrus.Error(err)
			http.Error(w, "something wrong decoding", http.StatusBadRequest)
			return
		}
	} else {
		storageURL, err = vt.CreateAndUploadSingleVoucher(formattedCodes[0])
		if err != nil {
			logrus.Error(err)
			http.Error(w, "something wrong decoding", http.StatusBadRequest)
			return
		}
	}
	logrus.Info(storageURL)
	//4. send e-mail to client
	w.WriteHeader(http.StatusOK)
	return
}
