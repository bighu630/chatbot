package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

const dbPath = "../quotations.db"

var jsonPath = "./Pop-Sentences/cp.json"
var level = "cp"

func main() {
	// JSON 数据（字符串数组）
	// read json file
	jsondata, err := os.ReadFile(jsonPath)
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
	var stringData []string
	if err = json.Unmarshal(jsondata, &stringData); err != nil {
		fmt.Println(err)
		panic(err)
	}

	// 打开数据库连接
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// 循环插入数据
	for _, row := range stringData {
		// 使用 `db.Exec` 插入数据并获取 last insert row ID
		result, err := db.Exec("INSERT INTO main(text, level) VALUES (?, ?)", row, level)
		if err != nil {
			log.Fatal(err)
		}
		// 获取插入行的 ID
		id, err := result.LastInsertId()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("插入数据成功，ID: %d\n", id)
	}

	fmt.Println("JSON 数据已成功插入 SQLite 数据库。")
}
