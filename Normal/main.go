package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// โครงสร้างข้อมูลของผู้เล่น (Player) ที่ใช้เก็บข้อมูลผู้เล่น เช่น ชื่อผู้เล่น และตำแหน่งในโลก
type Player struct {
	Username string  `json:"Username"` // ชื่อผู้เล่น
	X        float64 `json:"X"`        // พิกัด X ของผู้เล่น
	Y        float64 `json:"Y"`        // พิกัด Y ของผู้เล่น
	Z        float64 `json:"Z"`        // พิกัด Z ของผู้เล่น
}

// ตัวแปรที่ใช้สำหรับการจัดการการเข้าถึงข้อมูลที่ใช้ร่วมกัน (Shared Resources) ด้วย Mutex
var (
	players       = make(map[string]Player)        // เก็บข้อมูลผู้เล่นทั้งหมด
	playersMu     sync.Mutex                       // ใช้ล็อกเพื่อให้การเข้าถึงข้อมูลใน map `players` เป็นไปอย่างปลอดภัย
	connections   = make(map[*websocket.Conn]bool) // เก็บการเชื่อมต่อ WebSocket
	connectionsMu sync.Mutex                       // ใช้ล็อกเพื่อให้การเข้าถึงข้อมูลใน map `connections` เป็นไปอย่างปลอดภัย
)

// WebSocket Upgrader ใช้สำหรับอัพเกรดการเชื่อมต่อ HTTP ให้เป็น WebSocket
var upgrader = websocket.Upgrader{
	// ตรวจสอบแหล่งที่มาของคำขอ (Origins) ซึ่งในที่นี้อนุญาตให้เชื่อมต่อจากทุกแหล่ง
	CheckOrigin: func(r *http.Request) bool {
		return true // อนุญาตให้ทุกแหล่งที่มาเชื่อมต่อ
	},
}

// handleConnection ใช้สำหรับจัดการการเชื่อมต่อ WebSocket สำหรับแต่ละการเชื่อมต่อ
func handleConnection(conn *websocket.Conn) {
	defer conn.Close() // เมื่อเสร็จสิ้นการทำงานให้ปิดการเชื่อมต่อ

	// เพิ่มการเชื่อมต่อใหม่ใน map connections
	connectionsMu.Lock()
	connections[conn] = true
	connectionsMu.Unlock()

	var player Player // ตัวแปรเพื่อเก็บข้อมูลของผู้เล่น

	// อ่านข้อความจากการเชื่อมต่อในลูปต่อเนื่อง
	for {
		// อ่านข้อความจากการเชื่อมต่อ WebSocket
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("Error reading message:", err)
			break // ถ้ามีข้อผิดพลาดให้หยุดการทำงาน
		}

		// แปลงข้อความที่ได้รับให้เป็นข้อมูลของผู้เล่น
		if err := json.Unmarshal(msg, &player); err != nil {
			log.Println("Invalid JSON:", err) // ถ้าแปลงข้อมูลไม่ได้ แสดงข้อผิดพลาด
			continue                          // ข้ามการประมวลผลในรอบนี้ไป
		}

		// ตรวจสอบว่าชื่อผู้เล่นไม่ว่างเปล่า
		if player.Username == "" {
			log.Println("Invalid Player data: Username is empty") // ถ้าชื่อผู้เล่นว่าง
			continue                                              // ข้ามการประมวลผลในรอบนี้ไป
		}

		// Log the player's login details
		log.Printf("Player %s logged in from %s\n", player.Username, conn.RemoteAddr())

		// อัปเดตข้อมูลของผู้เล่นใน map `players` ด้วยการใช้ Mutex เพื่อให้การเข้าถึงข้อมูลปลอดภัย
		playersMu.Lock()
		players[player.Username] = player
		playersMu.Unlock()

		// แจ้งการอัปเดตข้อมูลของผู้เล่นให้กับผู้เล่นคนอื่น ๆ ผ่านการกระจายข้อมูล
		broadcastPlayerUpdate(player)
	}

	// เมื่อการเชื่อมต่อสิ้นสุดลง ให้ลบการเชื่อมต่อนั้นออกจาก map `connections`
	connectionsMu.Lock()
	delete(connections, conn)
	connectionsMu.Unlock()

	// แจ้งเตือนผู้เล่นคนอื่น ๆ เกี่ยวกับการตัดการเชื่อมต่อของผู้เล่นนี้
	broadcastDisconnect(player.Username)
	log.Println("Connection closed:", conn.RemoteAddr())
}

// broadcastPlayerUpdate ส่งข้อมูลของผู้เล่นที่อัปเดตให้กับทุกการเชื่อมต่อ WebSocket ที่เปิดอยู่
func broadcastPlayerUpdate(player Player) {
	connectionsMu.Lock() // ล็อกการเข้าถึง map `connections`
	defer connectionsMu.Unlock()

	// ส่งข้อมูลผู้เล่นให้กับทุกการเชื่อมต่อ
	for conn := range connections {
		if err := conn.WriteJSON(player); err != nil {
			log.Println("Error sending player data:", err) // ถ้ามีข้อผิดพลาดในการส่งข้อมูล
		}
	}
}

// broadcastDisconnect ส่งข้อความการตัดการเชื่อมต่อไปยังทุกการเชื่อมต่อ WebSocket
func broadcastDisconnect(username string) {
	// สร้างข้อความแจ้งการตัดการเชื่อมต่อ
	disconnectMsg := map[string]string{
		"action":   "disconnect", // การกระทำที่เกิดขึ้นคือการตัดการเชื่อมต่อ
		"username": username,     // ชื่อผู้เล่นที่ตัดการเชื่อมต่อ
	}

	connectionsMu.Lock() // ล็อกการเข้าถึง map `connections`
	defer connectionsMu.Unlock()

	// ส่งข้อความการตัดการเชื่อมต่อให้กับทุกการเชื่อมต่อ
	for conn := range connections {
		if err := conn.WriteJSON(disconnectMsg); err != nil {
			log.Println("Error sending disconnect message:", err) // ถ้ามีข้อผิดพลาดในการส่งข้อความ
		}
	}
}

// main ฟังก์ชันหลักที่ใช้เริ่มต้นเซิร์ฟเวอร์ HTTP และจัดการคำขอ WebSocket
func main() {
	// ตั้งค่าให้เซิร์ฟเวอร์ HTTP รับคำขอที่เส้นทาง /ws
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		// อัพเกรดการเชื่อมต่อ HTTP เป็น WebSocket
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("Error upgrading connection:", err) // ถ้ามีข้อผิดพลาดในการอัพเกรดการเชื่อมต่อ
			return
		}

		// เรียกฟังก์ชัน handleConnection เพื่อจัดการการเชื่อมต่อ WebSocket นี้
		handleConnection(conn)
	})

	// เริ่มต้นเซิร์ฟเวอร์ HTTP ที่พอร์ต 8080
	log.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil)) // เริ่มเซิร์ฟเวอร์และจัดการคำขอ
}
