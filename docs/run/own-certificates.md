---
title: Your own certificate authority
description: Make HolyArrs trust HTTPS addresses signed by your own certificate authority, such as https://torrents.home.lan.
level: Expert
sidebar: { order: 12 }
---

If your services use certificates from your own certificate authority (CA), HolyArrs needs that CA's root certificate. Without it, an `https://` address such as `https://torrents.home.lan` fails with a certificate error.

## Add your root certificate

1. Make a folder `certs` inside your HolyArrs config folder.
2. Put your root certificate in it as a PEM file, for example `root_ca.crt`. One file can hold several certificates, and the folder can hold several files.
3. Restart HolyArrs.

With Docker you can also mount only that file:

```yaml
volumes:
  - /docker/holyarrs/config:/config
  - /docker/tls/root_ca.crt:/config/certs/root_ca.crt:ro
```

The log shows one line at start:

```
holyarrs: trusting 1 of your own certificate(s) from /config/certs
```

HolyArrs then trusts your CA for every HTTPS call it makes: Downloaders, Sources, media servers, alerts and metadata. The usual public certificates keep working.

## Good to know

- The file must be PEM: text that starts with `-----BEGIN CERTIFICATE-----`. Convert a `.der` or `.cer` file first, for example with `openssl x509 -inform der -in root_ca.cer -out root_ca.crt`.
- Add the root (or intermediate) CA, not the certificate of one service.
- If you set `SSL_CERT_FILE` or `SSL_CERT_DIR` yourself, HolyArrs leaves the `certs` folder alone and uses what you set.
- The `certs` folder works on Linux and in Docker. On Windows and macOS, add the root certificate to the system's own certificate store instead; HolyArrs uses that store.
