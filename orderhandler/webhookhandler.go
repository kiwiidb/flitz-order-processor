package orderhandler

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"

	"github.com/gorilla/schema"
	"github.com/kiwiidb/bliksem-library/opennode"
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
var on *opennode.OpenNode
var ms *utils.MailSender

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
	err = vt.InitFirebase()
	if err != nil {
		logrus.Fatal(err)
	}

	//init database
	tdb = &tokendb.TokenDB{}
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
	err = m.Load(on)
	m.PrintEnvs(on)
	if err != nil {
		logrus.Fatal(err)
	}

	ms.Init()

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
	err = decoder.Decode(&order, r.URL.Query())
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
	if !utils.ValidMAC([]byte(whrb.ID), []byte(whrb.HashedOrder), []byte(on.APIKey)) {
		//invalid
		logrus.Error(fmt.Errorf("webhook called with wrong token %v %v", whrb, r))
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
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
	templateFilename := fmt.Sprintf("voucher_%d.png", order.Value)
	err = vt.DownloadTemplate(templateFilename)
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
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
	localFile := fmt.Sprintf("/tmp/%s", path.Base(storageURL))
	err = ms.DownloadVoucher(storageURL, localFile)
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong ", http.StatusInternalServerError)
		return
	}
	err = ms.SendMail(order.Email, emailBody, localFile)
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong ", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	return
}

var emailBody = `
Hello there!
You have received a Flitz voucher. 
Use your favourite LNURL-enabled wallet to redeem it.

Kind regards,

The Flitz team.
`
