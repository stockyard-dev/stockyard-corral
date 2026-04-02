package main
import ("fmt";"log";"net/http";"os";"github.com/stockyard-dev/stockyard-corral/internal/server";"github.com/stockyard-dev/stockyard-corral/internal/store")
func main(){port:=os.Getenv("PORT");if port==""{port="8760"};dataDir:=os.Getenv("DATA_DIR");if dataDir==""{dataDir="./corral-data"}
db,err:=store.Open(dataDir);if err!=nil{log.Fatalf("corral: %v",err)};defer db.Close();srv:=server.New(db)
fmt.Printf("\n  Corral — webhook inbox\n  Dashboard:  http://localhost:%s/ui\n  API:        http://localhost:%s/api\n\n",port,port)
log.Printf("corral: listening on :%s",port);log.Fatal(http.ListenAndServe(":"+port,srv))}
