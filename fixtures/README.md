Generated using `openssl`

First create a CA cert and key by
1. Generating a CSR for the CA. `openssl req -new -newkey rsa:4096 -nodes -out ca.csr -keyout ca-priv.key -sha256`
2. Self sign the CSR to create a self signed certificate. `openssl x509 -signkey ca-priv.key -days 365 -req -in ca.csr -out ca-cert.pem -sha256`
3. Create CSR for the server and clients using the command in bullet 1.
4. Sign the CSR's using the CA with `openssl x509 -req -days 365 -in server.csr -CA ca-cert.pem -CAkey ca-priv.key -out server-cert.pem -set_serial 01 -sha256`

CSR = `certificate sign request`
