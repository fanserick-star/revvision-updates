#!/usr/bin/env bash
# One VERSION file, hashes from final binary bytes; NEVER hand-type checksums.
set -euo pipefail
cd "$(dirname "$0")"
SOURCE="$(pwd)"
VERSION="$(tr -d '\r\n ' < VERSION)"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'VERSION inválida'; exit 1; }
SIGNING_KEY="${REVVISION_SIGNING_KEY:?Falta ruta de clave de firma offline REVVISION_SIGNING_KEY}"
[[ -f "$SIGNING_KEY" ]] || { echo 'Clave no existe'; exit 1; }
DEST="${1:-./dist}"
mkdir -p "$DEST"
DEST="$(cd "$DEST" && pwd)"
PUBLIC_KEY="$(go run ./cmd/signrelease pubkey "$SIGNING_KEY")"
export GOOS=windows GOARCH=amd64 CGO_ENABLED=0
LDFLAGS_APP="-s -w -H windowsgui -X main.appVersion=$VERSION"
go build -trimpath -ldflags "$LDFLAGS_APP" -o "$DEST/RevVisionApp.exe" .
(cd launcher && go build -trimpath -ldflags "-s -w -H windowsgui -X main.updatePublicKey=$PUBLIC_KEY -X main.launcherBuildVersion=$VERSION" -o "$DEST/RevVisionLauncher.exe" .)
go build -trimpath -ldflags '-s -w -H windowsgui' -o "$DEST/LauncherUpdater.exe" ./cmd/launcherhelper
python3 - "$DEST" "$VERSION" <<'PY'
import hashlib,json,pathlib,sys
p=pathlib.Path(sys.argv[1]);v=sys.argv[2]
h=lambda x:hashlib.sha256((p/x).read_bytes()).hexdigest()
m={'app_version':v,'app_name':'RevVisionApp.exe','app_sha256':h('RevVisionApp.exe'),'launcher_sha256':h('RevVisionLauncher.exe'),'helper_sha256':h('LauncherUpdater.exe'),'channel':'pruebas','schema_version':2}
(p/'release.json').write_text(json.dumps(m,indent=2,ensure_ascii=False)+'\n',encoding='utf8')
PY
PACKAGE="RevVision_App_${VERSION}_ACTUALIZACION.zip"
rm -f "$DEST/$PACKAGE" "$SOURCE/installer/bundle.zip"
(cd "$DEST" && zip -q -9 "$PACKAGE" RevVisionApp.exe RevVisionLauncher.exe LauncherUpdater.exe release.json)
URL="https://github.com/fanserick-star/revvision-updates/releases/download/v${VERSION}/${PACKAGE}"
GOOS='' GOARCH='' CGO_ENABLED='' go run ./cmd/signrelease sign "$SIGNING_KEY" "$VERSION" "$URL" "$DEST/$PACKAGE" "$DEST/RevVisionApp.exe" "$DEST/RevVisionLauncher.exe" "$DEST/LauncherUpdater.exe" "$DEST/update.json"
# IMPORTANT: verify all signed metadata and ZIP contents BEFORE building installable.
python3 - "$DEST" "$VERSION" <<'PY'
import hashlib,zipfile,json,sys,pathlib
p=pathlib.Path(sys.argv[1]);v=sys.argv[2]
m=json.loads((p/'update.json').read_text());rel=json.loads((p/'release.json').read_text())
assert m['version']==rel['app_version']==v
with zipfile.ZipFile(p/f'RevVision_App_{v}_ACTUALIZACION.zip') as z:
 assert sorted(z.namelist())==['LauncherUpdater.exe','RevVisionApp.exe','RevVisionLauncher.exe','release.json']
 for name,key in [('RevVisionApp.exe','app_sha256'),('RevVisionLauncher.exe','launcher_sha256'),('LauncherUpdater.exe','helper_sha256')]:
  h=hashlib.sha256(z.read(name)).hexdigest()
  assert h==m[key]==rel[key],name
 assert json.loads(z.read('release.json'))==rel
PY
mkdir -p "$SOURCE/installer"
(cd "$DEST" && zip -q -9 "$SOURCE/installer/bundle.zip" RevVisionApp.exe RevVisionLauncher.exe LauncherUpdater.exe release.json)
go build -trimpath -ldflags '-s -w -H windowsgui' -o "$DEST/INSTALAR_REVVISION_${VERSION}_BLINDADO.exe" ./installer
rm -f "$SOURCE/installer/bundle.zip"
echo "PAQUETE FIRMADO $VERSION generado, firmado y auditado. No publicado automáticamente."
