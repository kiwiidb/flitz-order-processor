**Flitz-order-processor**

Route '/store_batch'

Admin authentication required. Take a list of voucher codes and store the templated vouchers in storage.
Also return a signed url where they can be found


Route '/webhook'
Webhook to process incoming orders from opennode. Add vouchers to db, add vouchers to Firebase Storage, send e-mail with link/voucher to customer