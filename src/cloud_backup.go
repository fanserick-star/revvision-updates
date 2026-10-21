package main
// Optional encrypted backup to a clinic-owned Neon database. The live local
// JSON database remains authoritative. No multi-computer editing or plaintext
// clinical records are transmitted.
import(
 "context";"crypto/aes";"crypto/cipher";"crypto/rand";"crypto/sha256";"encoding/base64";"encoding/hex";"encoding/json";"errors";"net/http";"os";"path/filepath";"regexp";"strings";"sync";"time"
)
const rvMaxBackupBytes=8<<20
const rvIdleAfter=8*time.Minute
const rvMinimumBackup=15*time.Minute
var rvHashPattern=regexp.MustCompile(`^[0-9a-f]{64}$`)
type rvEncrypted struct{Format int `json:"format"`; Nonce string `json:"nonce"`; Data string `json:"data"`}
type rvBackupMeta struct{Fingerprint string `json:"fingerprint"`;LastSuccess string `json:"last_success"`;Target string `json:"target"`}
var rvBackupState=struct{sync.Mutex; LastActivity time.Time;LastAttempt time.Time;LastError string}{LastActivity:time.Now()}

func rvBackupKey()(key []byte,err error){
 p,e:=rvPath("neon-backup-key.dpapi");if e!=nil{return nil,e}
 data,e:=os.ReadFile(p);if e!=nil{return nil,e}
 key,e=rvReveal(data)
 if e!=nil||len(key)!=32{return nil,errors.New("No se pudo descifrar la llave de respaldo de esta cuenta Windows")}
 return key,nil
}
func rvBackupReady()bool{p,e:=rvPath("neon-backup-confirmed");if e!=nil{return false};_,e=os.Stat(p);return e==nil}
func rvSeal(data,key []byte)(string,error){
 if len(data)==0||len(data)>rvMaxBackupBytes{return "",errors.New("El archivo supera 8 MB. No se subió ningún dato")}
 c,e:=aes.NewCipher(key);if e!=nil{return "",e};g,e:=cipher.NewGCM(c);if e!=nil{return "",e}
 nonce:=make([]byte,g.NonceSize());if _,e=rand.Read(nonce);e!=nil{return "",e}
 out,_:=json.Marshal(rvEncrypted{Format:1,Nonce:base64.RawStdEncoding.EncodeToString(nonce),Data:base64.RawStdEncoding.EncodeToString(g.Seal(nil,nonce,data,[]byte("revvision-backup-v1")))})
 return string(out),nil
}
func rvUnseal(payload string,key []byte)([]byte,error){
 if len(payload)>rvMaxBackupBytes*2{return nil,errors.New("Respaldo remoto demasiado grande")}
 var item rvEncrypted;if json.Unmarshal([]byte(payload),&item)!=nil||item.Format!=1{return nil,errors.New("Formato de respaldo no reconocido")}
 n,e:=base64.RawStdEncoding.DecodeString(item.Nonce);if e!=nil{return nil,e}
 raw,e:=base64.RawStdEncoding.DecodeString(item.Data);if e!=nil{return nil,e}
 c,e:=aes.NewCipher(key);if e!=nil{return nil,e};g,e:=cipher.NewGCM(c);if e!=nil{return nil,e}
 if len(n)!=g.NonceSize(){return nil,errors.New("Nonce inválido")}
 data,e:=g.Open(nil,n,raw,[]byte("revvision-backup-v1"))
 if e!=nil{return nil,errors.New("Llave incorrecta o respaldo alterado")}
 return data,nil
}
func rvReadMeta()rvBackupMeta{
 p,e:=rvPath("neon-backup-state.json");if e!=nil{return rvBackupMeta{}}
 data,e:=os.ReadFile(p);if e!=nil{return rvBackupMeta{}}
 var meta rvBackupMeta;_ = json.Unmarshal(data,&meta);return meta
}
func rvWriteMeta(meta rvBackupMeta)error{
 p,e:=rvPath("neon-backup-state.json");if e!=nil{return e}
 b,e:=json.Marshal(meta);if e!=nil{return e}
 t:=p+".tmp";if e=os.WriteFile(t,b,0600);e!=nil{return e};defer os.Remove(t)
 return rvReplace(t,p)
}
func rvDatabaseBytes(s *Store)([]byte,error){
 s.mu.Lock();defer s.mu.Unlock()
 b,e:=os.ReadFile(s.file)
 if os.IsNotExist(e){b,e=json.Marshal(s.data)}
 if e!=nil{return nil,e}
 if len(b)>rvMaxBackupBytes{return nil,errors.New("La base local supera el límite de respaldo cifrado de 8 MB")}
 return b,nil
}
func rvSendBackup(ctx context.Context,s *Store)(bool,error){
 conn,e:=rvCredential();if e!=nil{return false,errors.New("Vincula Neon antes del respaldo")}
 key,e:=rvBackupKey();if e!=nil{return false,e}
 owner,e:=rvOwner();if e!=nil{return false,e}
 db,e:=rvDatabaseBytes(s);if e!=nil{return false,e}
 digest:=sha256.Sum256(db);hash:=hex.EncodeToString(digest[:])
 u,e:=rvParseURL(conn);if e!=nil{return false,e}
 target:=u.Hostname()+u.Path+"@"+owner
 meta:=rvReadMeta()
 if hash==meta.Fingerprint&&target==meta.Target{return false,nil}
 check,e:=rvQueryCall(ctx,conn,"SELECT owner_id FROM revvision.ownership WHERE id=1")
 if e!=nil{return false,e}
 if rvRow(check,"owner_id")!=owner{return false,errors.New("Neon pertenece a otra instalación; se bloqueó el envío")}
 ciphertext,e:=rvSeal(db,key);if e!=nil{return false,e}
 _,e=rvQueryCall(ctx,conn,`INSERT INTO revvision.encrypted_backups(owner_id,fingerprint,ciphertext) VALUES ($1,$2,$3) ON CONFLICT(owner_id,fingerprint) DO NOTHING`,owner,hash,ciphertext)
 if e!=nil{return false,e}
 // Old versions are pruned only after successful insertion. Keep last 8.
 _,_=rvQueryCall(ctx,conn,`DELETE FROM revvision.encrypted_backups WHERE id IN (SELECT id FROM revvision.encrypted_backups WHERE owner_id=$1 ORDER BY id DESC OFFSET 8)`,owner)
 _=rvWriteMeta(rvBackupMeta{Fingerprint:hash,Target:target,LastSuccess:time.Now().UTC().Format(time.RFC3339)})
 rvBackupState.Lock();rvBackupState.LastError="";rvBackupState.Unlock()
 return true,nil
}
func rvEmptyData(d Database)bool{return len(d.Patients)==0&&len(d.Consultations)==0&&len(d.Products)==0&&len(d.Orders)==0&&len(d.Payments)==0}
type rvRestoreRequest struct{ConnectionURL string `json:"connection_url"`;RecoveryKey string `json:"recovery_key"`}
func (s *Store) rvRestore(ctx context.Context,in rvRestoreRequest)error{
 s.mu.Lock();occupied:=!rvEmptyData(s.data);s.mu.Unlock()
 if occupied{return errors.New("Ya existen pacientes, consultas o ventas en esta PC: restauración bloqueada")}
 key,e:=base64.RawURLEncoding.DecodeString(strings.TrimSpace(in.RecoveryKey))
 if e!=nil||len(key)!=32{return errors.New("Llave de recuperación inválida")}
 if _,e=rvProbe(ctx,in.ConnectionURL);e!=nil{return e}
 rows,e:=rvQueryCall(ctx,in.ConnectionURL,"SELECT owner_id FROM revvision.ownership WHERE id=1")
 if e!=nil{return e};owner:=rvRow(rows,"owner_id")
 if len(owner)!=32{return errors.New("Esta base Neon no contiene una identidad Revvision válida")}
 if _,e=hex.DecodeString(owner);e!=nil{return errors.New("Identidad remota inválida")}
 rows,e=rvQueryCall(ctx,in.ConnectionURL,"SELECT fingerprint,ciphertext FROM revvision.encrypted_backups WHERE owner_id=$1 ORDER BY id DESC LIMIT 1",owner)
 if e!=nil{return e};if len(rows.Rows)==0{return errors.New("No hay copias cifradas para esta instalación")}
 fingerprint:=rvRow(rows,"fingerprint");if !rvHashPattern.MatchString(fingerprint){return errors.New("Huella de respaldo inválida")}
 data,e:=rvUnseal(rvRow(rows,"ciphertext"),key);if e!=nil{return e}
 digest:=sha256.Sum256(data);if hex.EncodeToString(digest[:])!=fingerprint{return errors.New("La verificación SHA-256 de la copia falló")}
 var database Database
 if e=json.Unmarshal(data,&database);e!=nil{return errors.New("Formato de base local incorrecto")}
 if database.Schema!=2||database.Patients==nil||database.Consultations==nil||database.Products==nil||database.Orders==nil||database.Payments==nil{return errors.New("Versión de respaldo incompatible; no se cambió nada")}
 p,e:=rvPath("neon-owner-id");if e!=nil{return e}
 // An initialized local identity must match the cloud owner. Never merge.
 if b,err:=os.ReadFile(p);err==nil&&strings.TrimSpace(string(b))!=owner{return errors.New("Identidad de esta PC distinta de la del respaldo")}
 credentialPath,_:=rvPath("neon-connection.dpapi")
 keyPath,_:=rvPath("neon-backup-key.dpapi")
 if _,err:=os.Stat(credentialPath);err==nil{return errors.New("Ya existe una cuenta Neon vinculada")}
 if _,err:=os.Stat(keyPath);err==nil{return errors.New("Ya existe una llave local; no se reemplazará")}
 encKey,e:=rvSecure(key);if e!=nil{return e}
 encURL,e:=rvSecure([]byte(in.ConnectionURL));if e!=nil{return e}
 s.mu.Lock();defer s.mu.Unlock()
 if !rvEmptyData(s.data){return errors.New("Se encontraron datos nuevos mientras se restauraba; operación cancelada")}
 if e=s.save(database);e!=nil{return e}
 if e=os.WriteFile(p,[]byte(owner+"\n"),0600);e!=nil{return e}
 if e=os.WriteFile(keyPath,encKey,0600);e!=nil{return e}
 if e=os.WriteFile(credentialPath,encURL,0600);e!=nil{return e}
 readyPath,_:=rvPath("neon-backup-confirmed")
 _=os.WriteFile(readyPath,[]byte("confirmed"),0600)
 _=rvWriteMeta(rvBackupMeta{Fingerprint:fingerprint,LastSuccess:time.Now().UTC().Format(time.RFC3339)})
 return nil
}
func rvDailyCheckpoint(filename string,prior []byte)error{
 if len(prior)==0{return nil}
 folder:=filepath.Join(filepath.Dir(filename),"recovery")
 if e:=os.MkdirAll(folder,0700);e!=nil{return e}
 name:=filepath.Join(folder,"revvision-"+time.Now().UTC().Format("2006-01-02")+".json")
 f,e:=os.OpenFile(name,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600)
 if os.IsExist(e){return nil};if e!=nil{return e}
 if _,e=f.Write(prior);e==nil{e=f.Sync()};cl:=f.Close()
 if e!=nil{_ =os.Remove(name);return e};return cl
}
func rvBackgroundBackup(ctx context.Context,s *Store){
 ticker:=time.NewTicker(time.Minute);defer ticker.Stop()
 for{
  select{case <-ctx.Done():return;case <-ticker.C:}
  rvBackupState.Lock()
  active:=time.Since(rvBackupState.LastActivity)<rvIdleAfter
  due:=time.Since(rvBackupState.LastAttempt)>=rvMinimumBackup
  if active&&due{rvBackupState.LastAttempt=time.Now()}
  rvBackupState.Unlock()
  if !active||!due||!rvBackupReady(){continue}
  if _,e:=rvBackupKey();e!=nil{continue}
  c,cancel:=context.WithTimeout(ctx,45*time.Second)
  _,e:=rvSendBackup(c,s);cancel()
  if e!=nil{rvBackupState.Lock();rvBackupState.LastError=e.Error();rvBackupState.Unlock()}
 }
}
func (s *Store) rvBackupHandler(w http.ResponseWriter,r *http.Request)bool{
 if !strings.HasPrefix(r.URL.Path,"/api/neon/backup/")&&r.URL.Path!="/api/neon/activity"{return false}
 if r.URL.Path=="/api/neon/activity"{
  if r.Method!="POST"{fail(w,405,"Método no permitido");return true}
  rvBackupState.Lock();rvBackupState.LastActivity=time.Now();rvBackupState.Unlock()
  jsonReply(w,200,map[string]bool{"ok":true});return true
 }
 switch r.URL.Path{
 case "/api/neon/backup/status":
  if r.Method!="GET"{fail(w,405,"Método no permitido");return true}
  _,e:=rvBackupKey();rvBackupState.Lock();idle:=time.Since(rvBackupState.LastActivity)>=rvIdleAfter;lastErr:=rvBackupState.LastError;rvBackupState.Unlock()
  jsonReply(w,200,map[string]interface{}{"enabled":e==nil,"ready":rvBackupReady(),"afk":idle,"last_backup":rvReadMeta().LastSuccess,"last_error":lastErr,"mode":"encrypted-backup-one-workstation"})
 case "/api/neon/backup/enable":
  if r.Method!="POST"{fail(w,405,"Método no permitido");return true}
  conn,e:=rvCredential();if e!=nil{fail(w,400,"Vincula primero Neon");return true}
  p,_:=rvPath("neon-backup-key.dpapi");if _,e=os.Stat(p);e==nil{fail(w,409,"Ya existe una llave de respaldo; no se sustituirá");return true}
  owner,e:=rvOwner();if e!=nil{fail(w,500,"No se pudo identificar esta PC");return true}
  ctx,cancel:=context.WithTimeout(r.Context(),35*time.Second);defer cancel()
  if e=rvProvision(ctx,conn,owner);e!=nil{fail(w,502,e.Error());return true}
  key:=make([]byte,32);if _,e=rand.Read(key);e!=nil{fail(w,500,"No se pudo crear llave segura");return true}
  enc,e:=rvSecure(key);if e!=nil{fail(w,500,"No se pudo proteger la llave");return true}
  f,e:=os.OpenFile(p,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if e!=nil{fail(w,500,"No se pudo guardar llave");return true}
  _,e=f.Write(enc);if e==nil{e=f.Sync()};cl:=f.Close()
  if e!=nil||cl!=nil{fail(w,500,"Falló guardado de llave");return true}
  jsonReply(w,200,map[string]string{"recovery_key":base64.RawURLEncoding.EncodeToString(key),"message":"Guarda esta llave fuera de la PC. No se enviarán respaldos hasta que lo confirmes."})
 case "/api/neon/backup/key":
  if r.Method!="POST"{fail(w,405,"Método no permitido");return true}
  key,e:=rvBackupKey();if e!=nil{fail(w,404,"No se encontró la llave");return true}
  jsonReply(w,200,map[string]string{"recovery_key":base64.RawURLEncoding.EncodeToString(key)})
 case "/api/neon/backup/confirm":
  if r.Method!="POST"{fail(w,405,"Método no permitido");return true}
  if _,e:=rvBackupKey();e!=nil{fail(w,409,"Activa primero la llave");return true}
  p,_:=rvPath("neon-backup-confirmed");if e:=os.WriteFile(p,[]byte("confirmed"),0600);e!=nil{fail(w,500,"No se pudo guardar confirmación");return true}
  jsonReply(w,200,map[string]bool{"ready":true})
 case "/api/neon/backup/now":
  if r.Method!="POST"{fail(w,405,"Método no permitido");return true}
  if !rvBackupReady(){fail(w,409,"Confirma antes que guardaste la llave de recuperación");return true}
  rvBackupState.Lock();if time.Since(rvBackupState.LastAttempt)<15*time.Second{rvBackupState.Unlock();fail(w,429,"Espera unos segundos antes de repetir");return true}
  rvBackupState.LastAttempt=time.Now();rvBackupState.Unlock()
  ctx,cancel:=context.WithTimeout(r.Context(),45*time.Second);defer cancel()
  changed,e:=rvSendBackup(ctx,s);if e!=nil{fail(w,502,e.Error());return true}
  jsonReply(w,200,map[string]interface{}{"uploaded":changed,"message":"Neon confirmó la copia cifrada. Tu base local no cambió"})
 case "/api/neon/backup/restore":
  if r.Method!="POST"{fail(w,405,"Método no permitido");return true}
  var request rvRestoreRequest
  if e:=receive(r,&request);e!=nil{fail(w,400,"Datos de recuperación incorrectos");return true}
  ctx,cancel:=context.WithTimeout(r.Context(),45*time.Second);defer cancel()
  if e:=s.rvRestore(ctx,request);e!=nil{fail(w,409,e.Error());return true}
  jsonReply(w,200,map[string]string{"message":"Datos restaurados en esta instalación vacía. Reinicia Revvision para verificar"})
 default:return false
 }
 return true
}
