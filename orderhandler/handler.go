package orderhandler

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"os"

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
	Price    int // Value and Price are sometimes different
	Amt      int
	Currency string
	Email    string
}

//Transaction contains information about on-chain tx received by opennode
type Transaction struct {
	Address       string `schema:"address"`
	CreatedAt     int    `schema:"created_at"`
	SettledAt     int    `schema:"settled_at"`
	TransactionID string `schema:"tx"`
	Status        string `schema:"status"`
	Amount        int    `schema:"amount"`
}

//WebHookRequestBody to check authenticity of OpenNode request
type WebHookRequestBody struct {
	HashedOrder string        `schema:"hashed_order"`
	ID          string        `schema:"id"`
	CallBackURL string        `schema:"callback_url"`
	SuccesURL   string        `schema:"success_url"`
	Status      string        `schema:"status"`
	OrderID     string        `schema:"order_id"`
	Description string        `schema:"description"`
	Price       string        `schema:"price"`
	Fee         string        `schema:"fee"`
	AutoSettle  string        `schema:"auto_settle"`
	MissingAmt  int           `schema:"missing_amt"`
	Transaction []Transaction `schema:"transactions"`
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
	m = multiconfig.EnvironmentLoader{}
	err = tdb.Initialize()
	if err != nil {
		logrus.Fatal(err)
	}
	//TODO deftige init voor alle library functions
	on = &opennode.OpenNode{}
	on.APIKey = os.Getenv("OPENNODE_APIKEY")
	ms = &utils.MailSender{}
	ms.Init()

}

//WebhookHandler to be called by Opennode
func WebhookHandler(w http.ResponseWriter, r *http.Request) {
	decoder := schema.NewDecoder()
	decoder.IgnoreUnknownKeys(true)
	order := Order{}
	whrb := WebHookRequestBody{}
	err := decoder.Decode(&order, r.URL.Query())
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
	logrus.WithField("Order", order).Info("Starting to process order")
	err = r.ParseForm()
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
	err = decoder.Decode(&whrb, r.PostForm)
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
	if !utils.ValidMAC([]byte(whrb.ID), whrb.HashedOrder, []byte(on.APIKey)) {
		//invalid
		logrus.Error(fmt.Errorf("webhook called with wrong token %v %v", whrb, r))
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
	if whrb.Status != "paid" {
		logrus.WithField("order", order).WithField("whrb", whrb).Info("order not yet paid (on chain needs to confirm or might be underpaid)")
		w.WriteHeader(http.StatusOK)
		return
	}
	logrus.WithField("whrb", whrb).Info("Starting to process order")
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
	//TODO remove hardcoded string!
	formatString := os.Getenv("LNURL_TEMPLATE")
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
	var localFile string
	if order.Amt > 1 {
		templateFilename := fmt.Sprintf("voucher_%d_%s.png", order.Price, order.Currency)
		err = vt.DownloadVoucherTemplateAndLoadInMemory(templateFilename)
		if err != nil {
			logrus.Error(err)
			http.Error(w, "something wrong decoding", http.StatusBadRequest)
			return
		}
		storageURL, err = vt.CreateAndUploadZipFromCodes(formattedCodes, whrb.ID)
		if err != nil {
			logrus.Error(err)
			http.Error(w, "something wrong decoding", http.StatusBadRequest)
			return
		}
		localFile = "/tmp/vouchers.zip"
	} else {
		templateFilename := fmt.Sprintf("voucher_custom.png")
		err = vt.DownloadVoucherTemplateAndLoadInMemory(templateFilename)
		if err != nil {
			logrus.Error(err)
			http.Error(w, "something wrong decoding", http.StatusBadRequest)
			return
		}
		storageURL, err = vt.CreateAndUploadSingleVoucher(formattedCodes[0])
		if err != nil {
			logrus.Error(err)
			http.Error(w, "something wrong decoding", http.StatusBadRequest)
			return
		}
		localFile = "/tmp/voucher.png"
	}
	err = ms.DownloadVoucher(storageURL, localFile)
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong ", http.StatusInternalServerError)
		return
	}
	emailBody, err := createEmailBody(order, formattedCodes)
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
	logrus.WithField("Order", order).Info("Succesfully processed order")
	w.WriteHeader(http.StatusOK)
	return
}

func createEmailBody(order Order, formattedCodes []string) (string, error) {
	isSingleVoucher := (len(formattedCodes) == 1)
	type EmailBodyInfo struct {
		Currency  string
		Amount    int
		LNURL     string
		RedeemURL string
		Number    int
	}
	var ebi EmailBodyInfo
	var emailBodyTemplateFileName string
	if isSingleVoucher {
		emailBodyTemplateFileName = "single_voucher_email_template.html"
		ebi = EmailBodyInfo{
			Currency:  order.Currency,
			Amount:    order.Value,
			LNURL:     formattedCodes[0],
			RedeemURL: ms.RedeemURL,
		}
	} else {
		emailBodyTemplateFileName = "multiple_voucher_email_template.html"
		ebi = EmailBodyInfo{
			Currency: order.Currency,
			Amount:   order.Value,
			Number:   order.Amt,
		}
	}

	templateBytes, err := vt.DownloadTemplate(emailBodyTemplateFileName)
	if err != nil {
		return "", err
	}
	tmpl, err := template.New("emailbody").Parse(string(templateBytes))
	if err != nil {
		return "", err
	}
	var bb bytes.Buffer
	err = tmpl.Execute(&bb, ebi)
	if err != nil {
		return "", err
	}
	return bb.String(), nil
}
