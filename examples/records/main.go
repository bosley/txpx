package main

import (
	"fmt"
	"log"
	"time"

	"github.com/bosley/txpx/pkg/record"
)

func main() {
	userSchema := record.DefineSchema("user").
		RequiredString("id").
		RequiredString("username").
		String("email").
		Int("login_count").
		Bool("is_active").
		Time("created_at").
		Time("last_login").
		Bytes("session_data").
		Build()

	sessionSchema := record.DefineSchema("web_session").
		RequiredString("session_id").
		RequiredString("user_id").
		String("ip_address").
		Time("expires_at").
		Bool("is_authenticated").
		Bytes("csrf_token").
		Build()

	fmt.Println("=== User Record Example ===")
	user := userSchema.NewRecord()

	user.SetString("id", "user_123")
	user.SetString("username", "bosley")
	user.SetString("email", "bosley@insula.dev")
	user.SetInt("login_count", 42)
	user.SetBool("is_active", true)
	user.SetTime("created_at", time.Now())
	user.SetTime("last_login", time.Now().Add(-1*time.Hour))
	user.SetBytes("session_data", []byte("encrypted_session_token"))

	fmt.Printf("User ID: %s\n", user.GetString("id"))
	fmt.Printf("Username: %s\n", user.GetString("username"))
	if count, err := user.GetInt("login_count"); err == nil {
		fmt.Printf("Login count: %d\n", count)
	}
	if active, err := user.GetBool("is_active"); err == nil {
		fmt.Printf("Is active: %t\n", active)
	}
	if created, err := user.GetTime("created_at"); err == nil {
		fmt.Printf("Created: %s\n", created.Format("2006-01-02 15:04:05"))
	}

	fmt.Println("\n=== Insi K/V Storage Simulation ===")
	kvData, err := user.ToKV()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Storing to Insi K/V backend:")
	for _, item := range kvData {
		fmt.Printf("  %s: %s\n", item.Key, item.Value)
	}

	fmt.Println("\nLoading from Insi K/V backend:")
	loadedUser := userSchema.NewRecord()
	if err := loadedUser.FromKV(kvData); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Loaded username: %s\n", loadedUser.GetString("username"))
	if count, err := loadedUser.GetInt("login_count"); err == nil {
		fmt.Printf("Loaded login count: %d\n", count)
	}

	fmt.Println("\n=== Planar K/V Storage Simulation ===")
	planarKVData, err := user.ToPlanarKV()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Storing to Planar K/V backend:")
	for _, item := range planarKVData {
		fmt.Printf("  %s: %s\n", string(item.Key), string(item.Value))
	}

	fmt.Println("\nLoading from Planar K/V backend:")
	planarLoadedUser := userSchema.NewRecord()
	if err := planarLoadedUser.FromPlanarKV(planarKVData); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Planar loaded username: %s\n", planarLoadedUser.GetString("username"))
	if count, err := planarLoadedUser.GetInt("login_count"); err == nil {
		fmt.Printf("Planar loaded login count: %d\n", count)
	}

	fmt.Println("\n=== Web Session Example ===")
	session := sessionSchema.NewRecord()
	session.SetString("session_id", "sess_abc123")
	session.SetString("user_id", "user_123")
	session.SetString("ip_address", "192.168.1.100")
	session.SetTime("expires_at", time.Now().Add(24*time.Hour))
	session.SetBool("is_authenticated", true)
	session.SetBytes("csrf_token", []byte("random_csrf_token_here"))

	fmt.Printf("Session ID: %s\n", session.GetString("session_id"))
	fmt.Printf("User ID: %s\n", session.GetString("user_id"))
	if expires, err := session.GetTime("expires_at"); err == nil {
		fmt.Printf("Expires: %s\n", expires.Format("2006-01-02 15:04:05"))
	}

	fmt.Println("\n=== Map Conversion ===")
	userMap := user.ToMap()
	fmt.Println("User as map:")
	for k, v := range userMap {
		fmt.Printf("  %s: %v\n", k, v)
	}

	newUserFromMap := userSchema.NewRecord()
	if err := newUserFromMap.FromMap(userMap); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Reconstructed from map - username: %s\n", newUserFromMap.GetString("username"))

	fmt.Println("\n=== Schema Info ===")
	fmt.Printf("User schema: %s\n", user.Schema())
	fmt.Printf("Session schema: %s\n", session.Schema())
}
