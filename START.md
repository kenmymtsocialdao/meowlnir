export LD_LIBRARY_PATH=/usr/local/lib/libolm.so.3:$LD_LIBRARY_PATH

bash build.sh 

export AUTH="Authorization: Bearer pk434zki6ITYjZbOtRsp2mSndEaqlFQuL48dTzy8bJwLPPf4CxeJpYxjarRDrUVc"




curl -H "$AUTH" ec2-54-169-244-73.ap-southeast-1.compute.amazonaws.com:29339/_matrix/meowlnir/v1/bot/meowlnir_bot  -XPUT -d '{"displayname": "Administrator", "avatar_url": "mxc://matrix.org/NZGChxcCXbBvgkCNZTLXlpux"}'



curl -H "$AUTH" http://ec2-54-169-244-73.ap-southeast-1.compute.amazonaws.com:29339/_matrix/meowlnir/v1/bot/meowlnir3_bot/verify  -X POST -d '{"generate": true}'

{"recovery_key":"EsTo BNht L4ut XBrw ucmF HGEL d7Ju Nx5w A2cf RABb 8wP3 aEHe"}



curl -H "$AUTH" http://ec2-54-169-244-73.ap-southeast-1.compute.amazonaws.com:29339/_matrix/meowlnir/v1/bot/meowlnir002_bot/verify  -X POST -d '{"generate": true}'
{"recovery_key":"EsTf YTHR ePc1 P9wR 4bo5 eWEa dfX4 CYw5 wAsj S33p M9EX 8B5G"}




curl -H "$AUTH" "http://ec2-54-169-244-73.ap-southeast-1.compute.amazonaws.com:29339/_matrix/meowlnir/v1/management_room/\!QAfwDqrrrumLBBREHj:server.mtsocialdao.com"  -XPUT -d '{"bot_username":"meowlnir002_bot"}' -i

