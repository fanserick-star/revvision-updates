#!/usr/bin/env python3
"""Verify Rev Visión release bytes, versions, Ed25519 signatures and SHA-256."""
import base64,hashlib,io,json,os,re,subprocess,sys,tempfile,urllib.request,zipfile
from pathlib import Path
LIMIT=45*1024*1024
REPO="fanserick-star/revvision-updates"
def verify(tag, data):
    if not re.fullmatch(r"v\d{1,5}\.\d{1,5}\.\d{1,5}",tag): raise ValueError("Etiqueta inválida")
    version=tag[1:]
    m=json.loads((Path("candidates")/(tag+".json")).read_text())
    fmt=m.get("format")
    if fmt not in (1,2) or m.get("version")!=version: raise ValueError("Versión o formato incompatible")
    url=f"https://github.com/{REPO}/releases/download/{tag}/RevVision_App_{version}_ACTUALIZACION.zip"
    if m.get("package_url")!=url: raise ValueError("Dirección de paquete no autorizada")
    fields=["package_sha256","app_sha256"]+(["launcher_sha256","helper_sha256"] if fmt==2 else [])
    if any(not re.fullmatch("[a-f0-9]{64}",str(m.get(x,""))) for x in fields): raise ValueError("Hash mal formado")
    if fmt==1:
        payload=f"revvision-update-v1\n1\n{version}\n{url}\n{m['package_sha256']}\n{m['app_sha256']}\n"
    else:
        payload=(f"revvision-update-v2\n2\n{version}\n{url}\n{m['package_sha256']}\n{m['app_sha256']}\n"
                 f"{m['launcher_sha256']}\n{m['helper_sha256']}\n")
    with tempfile.TemporaryDirectory() as tmp:
        p=Path(tmp)
        (p/"payload").write_bytes(payload.encode())
        (p/"signature").write_bytes(base64.b64decode(m["signature"],validate=True))
        r=subprocess.run(["openssl","pkeyutl","-verify","-pubin","-inkey","keys/release_public.pem","-sigfile",str(p/"signature"),"-rawin","-in",str(p/"payload")],capture_output=True)
        if r.returncode: raise ValueError("Firma digital Ed25519 inválida")
    if not data or len(data)>LIMIT or hashlib.sha256(data).hexdigest()!=m["package_sha256"]:
        raise ValueError("ZIP inexistente, demasiado grande o SHA-256 incorrecto")
    expected={"RevVisionApp.exe","release.json"}
    if fmt==2: expected|={"RevVisionLauncher.exe","LauncherUpdater.exe"}
    with zipfile.ZipFile(io.BytesIO(data)) as z:
        entries=z.infolist()
        if len(entries)!=len(expected) or {x.filename for x in entries}!=expected or any(x.is_dir() or x.file_size>35*1024*1024 for x in entries):
            raise ValueError("Archivos del paquete no permitidos")
        r=json.loads(z.read("release.json"))
        if r.get("app_version")!=version or r.get("app_name")!="RevVisionApp.exe": raise ValueError("Release interno incorrecto")
        pairs=[("RevVisionApp.exe","app_sha256")]
        if fmt==2:pairs +=[("RevVisionLauncher.exe","launcher_sha256"),("LauncherUpdater.exe","helper_sha256")]
        for filename,key in pairs:
            h=hashlib.sha256(z.read(filename)).hexdigest()
            if h!=m[key] or h!=r.get(key): raise ValueError("SHA de "+filename+" no coincide")
    return m
def main():
    event=json.loads(Path(os.environ["GITHUB_EVENT_PATH"]).read_text())
    tag=event.get("release",{}).get("tag_name") or event.get("inputs",{}).get("tag","")
    if not tag:raise ValueError("Falta tag")
    version=tag[1:]
    url=f"https://github.com/{REPO}/releases/download/{tag}/RevVision_App_{version}_ACTUALIZACION.zip"
    req=urllib.request.Request(url,headers={"User-Agent":"RevVision-ReleaseVerifier"})
    with urllib.request.urlopen(req,timeout=90) as response: data=response.read(LIMIT+1)
    m=verify(tag,data)
    active=Path("update.json")
    if active.exists():
        old=json.loads(active.read_text())["version"]
        if tuple(map(int,version.split(".")))<=tuple(map(int,old.split("."))):
            print("Versión igual o anterior: se conserva el canal");return
    active.write_text(json.dumps(m,ensure_ascii=False,indent=2)+"\n")
    print("Release validada y activada:",tag)
if __name__=="__main__":
    try:main()
    except Exception as err:sys.exit("PUBLICACIÓN BLOQUEADA: "+str(err))
