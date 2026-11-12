#!/bin/sh
# Upgrade wizard: asks for the media shared folder again, already filled with
# the name saved at install, so an upgrade keeps the folder's permission.
share="data"
saved="/var/packages/holyarrs/var/media-share"
if [ -r "$saved" ]; then
    name="$(head -n 1 "$saved" | tr -d '\r"\')"
    [ -n "$name" ] && share="$name"
fi
cat > "${SYNOPKG_TEMP_LOGFILE:-/dev/stdout}" <<JSON
[
  {
    "step_title": "Media folder",
    "items": [
      {
        "type": "textfield",
        "desc": "The shared folder that holds your downloads and media. HolyArrs keeps read and write on it. Leave it as it is unless you moved your media.",
        "subitems": [
          {
            "key": "wizard_media_share",
            "desc": "Shared folder",
            "defaultValue": "$share",
            "validator": {
              "allowBlank": false
            }
          }
        ]
      }
    ]
  }
]
JSON
exit 0
