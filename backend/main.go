package main

import (
    "log"
    "net/http"
)

func main() {
    initDB()

    mux := http.NewServeMux()

    mux.HandleFunc("/api/register/options", registerOptionsHandler)
    mux.HandleFunc("/api/register/verify", registerVerifyHandler)
    mux.HandleFunc("/api/login/options", loginOptionsHandler)
    mux.HandleFunc("/api/login/verify", loginVerifyHandler)
    mux.HandleFunc("/api/logout", logoutHandler)
    mux.HandleFunc("/api/session", sessionHandler)

    fs := http.FileServer(http.Dir("./frontend"))
    mux.Handle("/", fs)

    log.Println("Server running on http://localhost:8080")
    log.Fatal(http.ListenAndServe(":8080", mux))
}
