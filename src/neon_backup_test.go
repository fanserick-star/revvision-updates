package main
import(
 "bytes";"context";"crypto/rand";"encoding/json";"net/http";"net/http/httptest";"os";"path/filepath";"strings";"testing"
)
func TestRVEncryptedBackupRoundTripAndBadKey(t *testing.T){
 key:=make([]byte,32);_,_ = rand.Read(key)
 plain:=[]byte(`{"patients":{"fake":{"full_name":"PACIENTE FICTICIO"}}}`)
 sealed,err:=rvSeal(plain,key);if err!=nil{t.Fatal(err)}
 if strings.Contains(sealed,"PACIENTE FICTICIO"){t.Fatal("Datos en claro filtrados al backup")}
 opened,err:=rvUnseal(sealed,key);if err!=nil||!bytes.Equal(opened,plain){t.Fatalf("Cifrado no recuperable: %v",err)}
 other:=make([]byte,32);_,_=rand.Read(other)
 if _,err=rvUnseal(sealed,other);err==nil{t.Fatal("Llave equivocada aceptada")}
 if _,err=rvSeal(make([]byte,rvMaxBackupBytes+1),key);err==nil{t.Fatal("Respaldo excesivo no bloqueado")}
 if _,err=rvUnseal(sealed+"ALGO",key);err==nil{t.Fatal("Respaldo alterado no bloqueado")}
}
func TestRVNeonURLMustRequireRealNeonTLS(t *testing.T){
 valid:="postgresql://a:secret@ep-fake.us-east-1.aws.neon.tech/neondb?sslmode=require"
 if _,err:=rvParseURL(valid);err!=nil{t.Fatal(err)}
 for _,s:=range []string{
  "postgresql://a:p@ep-fake.us-east-1.aws.neon.tech.evil.example/db?sslmode=require",
  "postgresql://a:p@evil.example/db?sslmode=require",
  "postgresql://a:p@ep-fake.neon.tech/db?sslmode=disable",
  "http://ep-fake.neon.tech/db",
  "postgresql://a:p@ep-fake.neon.tech:9999/db?sslmode=require",
  "postgresql://a:p@ep-fake.neon.tech/db?sslmode=require#fragment",
 }{if _,err:=rvParseURL(s);err==nil{t.Errorf("URL insegura aceptada: %s",s)}}
}
func TestRVNeonHTTPProtocolDoesNotEchoSecret(t *testing.T){
 const secret="postgresql://u:SU_PASSWORD@ep-fake.neon.tech/neondb?sslmode=require"
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Method!="POST"||r.Header.Get("Neon-Connection-String")!=secret||r.Header.Get("Neon-Raw-Text-Output")!="true"{t.Error("Faltan cabeceras Neon")}
  var q rvQueryRequest;if err:=json.NewDecoder(r.Body).Decode(&q);err!=nil{t.Error(err)}
  if q.Query!="SELECT $1::text AS name"||len(q.Params)!=1||q.Params[0]!="fake"{t.Errorf("SQL erróneo: %+v",q)}
  w.Header().Set("Content-Type","application/json")
  _,_=w.Write([]byte(`{"rows":[{"name":"fake"}],"rowCount":1}`))
 }))
 defer server.Close()
 rows,err:=rvQueryHTTP(context.Background(),server.Client(),server.URL,secret,"SELECT $1::text AS name","fake")
 if err!=nil||rvRow(rows,"name")!="fake"{t.Fatalf("Protocolo SQL falló: %v",err)}
 deny:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  w.WriteHeader(400);_,_=w.Write([]byte(secret))
 }))
 defer deny.Close()
 _,err=rvQueryHTTP(context.Background(),deny.Client(),deny.URL,secret,"SELECT 1")
 if err==nil||strings.Contains(err.Error(),"SU_PASSWORD"){t.Fatal("Error puede filtrar contraseña")}
}
func TestRVDailyRecoveryCheckpointImmutable(t *testing.T){
 f:=filepath.Join(t.TempDir(),"revvision-local.json")
 if err:=rvDailyCheckpoint(f,[]byte("safe"));err!=nil{t.Fatal(err)}
 dir:=filepath.Join(filepath.Dir(f),"recovery")
 entries,err:=os.ReadDir(dir);if err!=nil||len(entries)!=1{t.Fatalf("Sin copia diaria: %v",err)}
 dst:=filepath.Join(dir,entries[0].Name())
 if err=rvDailyCheckpoint(f,[]byte("altered"));err!=nil{t.Fatal(err)}
 value,err:=os.ReadFile(dst);if err!=nil||string(value)!="safe"{t.Fatal("Sobrescribió el punto de recuperación")}
}
func TestRVRestoreWithExistingPatientsBlocksBeforeNetwork(t *testing.T){
 s:=&Store{data:freshDatabase()}
 s.data.Patients["fake"]=Patient{ID:"fake",FullName:"PACIENTE FICTICIO"}
 if err:=s.rvRestore(context.Background(),rvRestoreRequest{});err==nil||!strings.Contains(err.Error(),"restauración bloqueada"){t.Fatalf("Sobrescritura no bloqueada: %v",err)}
}
