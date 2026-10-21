package main

// Optional Neon connection. Revvision's local database is authoritative and
// no patient or financial data is uploaded until an owner opts in to backups.
import(
 "bytes";"context";"crypto/rand";"embed";"encoding/hex";"encoding/json";"errors";"fmt";"io";"net/http";"net/url";"os";"path/filepath";"strings";"time"
)
//go:embed revvision_neon_schema.sql
var rvSQLSchema embed.FS
type rvRows struct{Rows []map[string]json.RawMessage `json:"rows"`}
type rvRequest struct{ConnectionURL string `json:"connection_url"`}
type rvQueryRequest struct{Query string `json:"query"`;Params []string `json:"params"`}
var rvHTTPClient=&http.Client{Timeout:12*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}

func rvParseURL(raw string)(*url.URL,error){
 if len(raw)<35||len(raw)>4000||strings.ContainsAny(raw," \t\r\n\x00"){return nil,errors.New("Copia la URL PostgreSQL completa de Neon")}
 u,e:=url.Parse(raw);if e!=nil{return nil,errors.New("URL de Neon inválida")}
 if u.Scheme!="postgres"&&u.Scheme!="postgresql"{return nil,errors.New("Neon requiere postgresql://")}
 host:=strings.ToLower(u.Hostname())
 if !strings.HasSuffix(host,".neon.tech")||strings.Contains(host,"..")||len(host)>250||strings.HasPrefix(host,"."){return nil,errors.New("El servidor debe pertenecer a neon.tech")}
 if u.Port()!=""&&u.Port()!="5432"{return nil,errors.New("Puerto Neon inválido")}
 if u.User==nil||u.User.Username()==""{return nil,errors.New("Falta usuario Neon")}
 if password,ok:=u.User.Password();!ok||password==""{return nil,errors.New("Falta contraseña Neon")}
 if strings.Trim(u.Path,"/")==""||strings.Contains(strings.TrimPrefix(u.Path,"/"),"/"){return nil,errors.New("Selecciona una base de datos Neon")}
 if u.Query().Get("sslmode")!="require"&&u.Query().Get("sslmode")!="verify-full"{return nil,errors.New("Agrega sslmode=require a la conexión")}
 if u.Fragment!=""{return nil,errors.New("URL Neon no debe incluir fragmento")}
 return u,nil
}
func rvQueryHTTP(ctx context.Context, client *http.Client,endpoint,conn,sql string,params ...string)(rvRows,error){
 body,_:=json.Marshal(rvQueryRequest{Query:sql,Params:params})
 req,e:=http.NewRequestWithContext(ctx,http.MethodPost,endpoint,bytes.NewReader(body));if e!=nil{return rvRows{},errors.New("Solicitud Neon inválida")}
 req.Header.Set("Content-Type","application/json")
 req.Header.Set("Neon-Connection-String",conn)
 req.Header.Set("Neon-Raw-Text-Output","true")
 res,e:=client.Do(req);if e!=nil{return rvRows{},errors.New("No fue posible conectarse a Neon mediante HTTPS")}
 defer res.Body.Close()
 if res.StatusCode<200||res.StatusCode>299{return rvRows{},fmt.Errorf("Neon rechazó la solicitud (HTTP %d). Revisa la cuenta y permisos",res.StatusCode)}
 raw,e:=io.ReadAll(io.LimitReader(res.Body,17<<20));if e!=nil{return rvRows{},errors.New("Respuesta Neon incompleta")}
 var rows rvRows
 if e=json.Unmarshal(raw,&rows);e!=nil||rows.Rows==nil{return rvRows{},errors.New("Respuesta inesperada de Neon")}
 return rows,nil
}
func rvQuery(ctx context.Context,conn,sql string,params ...string)(rvRows,error){
 u,e:=rvParseURL(conn);if e!=nil{return rvRows{},e}
 return rvQueryHTTP(ctx,rvHTTPClient,"https://"+u.Hostname()+"/sql",conn,sql,params...)
}
var rvQueryCall=rvQuery
func rvRow(rows rvRows,key string)string{
 if len(rows.Rows)==0{return ""}
 var out string;_ = json.Unmarshal(rows.Rows[0][key],&out);return out
}
func rvPath(name string)(string,error){d,e:=storeDir();if e!=nil{return "",e};return filepath.Join(d,name),nil}
func rvOwner()(string,error){
 p,e:=rvPath("neon-owner-id");if e!=nil{return "",e}
 b,e:=os.ReadFile(p);if e==nil{
  v:=strings.TrimSpace(string(b))
  if len(v)!=32{return "",errors.New("Identidad Neon local dañada")}
  if _,e=hex.DecodeString(v);e!=nil{return "",errors.New("Identidad Neon local dañada")}
  return v,nil
 }
 if !os.IsNotExist(e){return "",e}
 key:=make([]byte,16);if _,e=rand.Read(key);e!=nil{return "",e}
 id:=hex.EncodeToString(key)
 f,e:=os.OpenFile(p,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 if os.IsExist(e){return rvOwner()}
 if e!=nil{return "",e}
 _,e=f.WriteString(id+"\n");if e==nil{e=f.Sync()};cl:=f.Close();if e!=nil{return "",e};if cl!=nil{return "",cl}
 return id,nil
}
func rvCredential()(string,error){
 p,e:=rvPath("neon-connection.dpapi");if e!=nil{return "",e}
 b,e:=os.ReadFile(p);if e!=nil{return "",e}
 raw,e:=rvReveal(b);if e!=nil{return "",errors.New("No se pudo abrir credencial Neon en esta cuenta Windows")}
 if _,e=rvParseURL(string(raw));e!=nil{return "",e}
 return string(raw),nil
}
func rvSaveCredential(raw string)error{
 p,e:=rvPath("neon-connection.dpapi");if e!=nil{return e}
 b,e:=rvSecure([]byte(raw));if e!=nil{return e}
 tmp:=p+".tmp"
 if e=os.WriteFile(tmp,b,0600);e!=nil{return e}
 defer os.Remove(tmp)
 return rvReplace(tmp,p)
}
func rvProbe(ctx context.Context,raw string)(string,error){
 u,e:=rvParseURL(raw);if e!=nil{return "",e}
 rows,e:=rvQueryCall(ctx,raw,"SELECT current_database() AS dbname");if e!=nil{return "",e}
 db:=strings.TrimPrefix(u.Path,"/")
 if rvRow(rows,"dbname")!=db{return "",errors.New("La base de datos Neon no coincide")}
 return db,nil
}
func rvProvision(ctx context.Context,conn,owner string)error{
 found,e:=rvQueryCall(ctx,conn,"SELECT to_regclass('revvision.ownership')::text AS table_name");if e!=nil{return e}
 if rvRow(found,"table_name")!=""{
  rows,err:=rvQueryCall(ctx,conn,"SELECT owner_id FROM revvision.ownership WHERE id=1");if err!=nil{return err}
  if len(rows.Rows)>0&&rvRow(rows,"owner_id")!=owner{return errors.New("Esta cuenta Neon ya pertenece a otra instalación Revvision")}
 }
 sqlb,e:=rvSQLSchema.ReadFile("revvision_neon_schema.sql");if e!=nil{return e}
 for _,statement:=range strings.Split(string(sqlb),";"){
  var lines []string
  for _,line:=range strings.Split(statement,"\n"){l:=strings.TrimSpace(line);if l!=""&&!strings.HasPrefix(l,"--"){lines=append(lines,line)}}
  sql:=strings.TrimSpace(strings.Join(lines,"\n"));if sql==""{continue}
  if _,e=rvQueryCall(ctx,conn,sql);e!=nil{return e}
 }
 if _,e=rvQueryCall(ctx,conn,"INSERT INTO revvision.ownership(id,owner_id) VALUES(1,$1) ON CONFLICT(id) DO NOTHING",owner);e!=nil{return e}
 rows,e:=rvQueryCall(ctx,conn,"SELECT owner_id FROM revvision.ownership WHERE id=1")
 if e!=nil{return e};if rvRow(rows,"owner_id")!=owner{return errors.New("Base Neon pertenece a otra instalación")}
 return nil
}
func (s *Store) neonHandler(w http.ResponseWriter,r *http.Request)bool{
 if !strings.HasPrefix(r.URL.Path,"/api/neon/") {return false}
 switch r.URL.Path{
 case "/api/neon/status":
  if r.Method!="GET"{fail(w,405,"Método no permitido");return true}
  raw,e:=rvCredential()
  if e!=nil{jsonReply(w,200,map[string]interface{}{"linked":false,"mode":"local","message":"Guardado local, sin cuenta Neon"});return true}
  u,_:=rvParseURL(raw)
  jsonReply(w,200,map[string]interface{}{"linked":true,"host":u.Hostname(),"database":strings.TrimPrefix(u.Path,"/"),"mode":"optional-backup","message":"Cuenta vinculada; datos locales prioritarios"})
 case "/api/neon/test","/api/neon/link":
  if r.Method!="POST"{fail(w,405,"Método no permitido");return true}
  var request rvRequest;if e:=receive(r,&request);e!=nil{fail(w,400,"Conexión Neon inválida");return true}
  ctx,cancel:=context.WithTimeout(r.Context(),30*time.Second);defer cancel()
  db,e:=rvProbe(ctx,request.ConnectionURL);if e!=nil{fail(w,502,e.Error());return true}
  if r.URL.Path=="/api/neon/link"{
   owner,err:=rvOwner();if err!=nil{fail(w,500,"No se pudo generar identidad local");return true}
   if err=rvProvision(ctx,request.ConnectionURL,owner);err!=nil{fail(w,409,err.Error());return true}
   if err=rvSaveCredential(request.ConnectionURL);err!=nil{fail(w,500,"No fue posible guardar la conexión cifrada");return true}
  }
  jsonReply(w,200,map[string]interface{}{"ok":true,"database":db,"linked":r.URL.Path=="/api/neon/link","message":"Conexión Neon verificada"})
 case "/api/neon/check":
  if r.Method!="POST"{fail(w,405,"Método no permitido");return true}
  conn,e:=rvCredential();if e!=nil{fail(w,400,"Primero vincula Neon");return true}
  ctx,cancel:=context.WithTimeout(r.Context(),15*time.Second);defer cancel()
  db,e:=rvProbe(ctx,conn);if e!=nil{fail(w,502,e.Error());return true}
  jsonReply(w,200,map[string]interface{}{"online":true,"database":db})
 case "/api/neon/unlink":
  if r.Method!="POST"{fail(w,405,"Método no permitido");return true}
  p,e:=rvPath("neon-connection.dpapi");if e!=nil{fail(w,500,"Ruta local no disponible");return true}
  if e=os.Remove(p);e!=nil&&!os.IsNotExist(e){fail(w,500,"No se pudo desvincular la cuenta");return true}
  jsonReply(w,200,map[string]interface{}{"linked":false,"message":"Neon desvinculado solamente de esta PC; no se borraron respaldos"})
 default:
  return false
 }
 return true
}
