package main

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chai2010/webp"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

var storedClient *whatsmeow.Client

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		msgTimestamp := v.Message.MessageContextInfo.DeviceListMetadata.SenderTimestamp
		timeNow := time.Now().Unix()
		calc := timeNow - int64(*msgTimestamp)
		calcInSec := calc / 1000
		fmt.Println("Message received", calcInSec)
		if calcInSec <= 20 && v.Info.IsFromMe {
			err := SendMessageImage(storedClient, v.Info.Chat)
			if err != nil {
				fmt.Println(err)
			}
		} else {
			fmt.Println("Ignoring old message and message not from myself")
		}

	}

}

func main() {
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	container, err := sqlstore.New("sqlite3", "file:examplestore.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}
	deviceStore, err := container.GetFirstDevice()
	if err != nil {
		panic(err)
	}
	// clientLog := waLog.Stdout("Client", "DEBUG", true)
	client := whatsmeow.NewClient(deviceStore, nil)
	client.AddEventHandler(eventHandler)
	storedClient = client

	if client.Store.ID == nil {
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			panic(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				fmt.Println("QR code:", evt.Code)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		// Already logged in, just connect
		err = client.Connect()
		if err != nil {
			panic(err)
		}
	}

	// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	client.Disconnect()
}

func SendMessageImage(client *whatsmeow.Client, jid types.JID) error {
	allowedJID := []types.JID{
		types.NewJID("contanct_number", "s.whatsapp.net"), // contact number s.whatsapp.net
		types.NewJID("group_id", "g.us"), // group g.us
	}

	// check if jid is allowed
	var allowed bool
	for _, v := range allowedJID {
		if v.String() == jid.String() {
			allowed = true
		}
	}
	if !allowed {
		return fmt.Errorf("JID not allowed")
	}

	allImgs, err := os.ReadDir("stickers")
	if err != nil {
		return err
	}
	var allowedImage []string
	for _, v := range allImgs {
		if v.IsDir() {
			continue
		}
		allowedImage = append(allowedImage, "stickers/"+v.Name())
	}

	imagePath := allowedImage[rand.Intn(len(allowedImage))]
	imageFile, err := os.Open(imagePath)
	if err != nil {
		return err
	}
	defer imageFile.Close()

	_, format, err := image.DecodeConfig(imageFile)
	if err != nil {
		return err
	}

	_, err = imageFile.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	if format != "jpeg" && format != "jpg" {
		fmt.Printf("unsupported image format")
		return err
	}

	img, err := jpeg.Decode(imageFile)
	if err != nil {
		return err
	}
	webpImage, _ := webp.EncodeRGBA(img, *proto.Float32(1))

	// upload image and create message
	uploadImage, err := client.Upload(context.Background(), webpImage, whatsmeow.MediaImage)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	imageMsg := &waProto.StickerMessage{
		Mimetype:          proto.String(http.DetectContentType(webpImage)),
		Url:               proto.String(uploadImage.URL),
		FileSha256:        uploadImage.FileSHA256,
		FileEncSha256:     uploadImage.FileEncSHA256,
		MediaKey:          uploadImage.MediaKey,
		Height:            proto.Uint32(100),
		Width:             proto.Uint32(100),
		DirectPath:        proto.String(uploadImage.DirectPath),
		FileLength:        proto.Uint64(uint64(len(webpImage))),
		MediaKeyTimestamp: proto.Int64(now),
		FirstFrameLength:  proto.Uint32(1),
		FirstFrameSidecar: nil,
		IsAnimated:        proto.Bool(false),
		PngThumbnail:      nil,
	}

	msg := &waProto.Message{
		StickerMessage: imageMsg,
	}
	_, err = client.SendMessage(context.Background(), jid, msg)
	return err
}
