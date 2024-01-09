package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"path"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"context"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

func addLabel(img image.Image, x, y int, label string) image.Image {
	// 원래 이미지와 같은 사이즈로 새로운 이미지 생성
	rgba := image.NewRGBA(img.Bounds())
	// img의 모든 픽셀을 rgba에다 그대로 덮어 씌움
	draw.Draw(rgba, rgba.Bounds(), img, image.Point{0, 0}, draw.Src)
	// 색상을 생성합니다. RGBA에서 255, 0, 0, 255는 빨간색을 나타냅니다.
	col := color.RGBA{255, 0, 0, 255}
	// 텍스트를 그릴 시작점을 설정합니다. x와 y는 픽셀 단위의 좌표입니다.
	point := fixed.Point26_6{fixed.Int26_6(x * 64), fixed.Int26_6(y * 64)}

	d := &font.Drawer{
		Dst:  rgba,                  // Dst는 목적지 이미지를 지정합니다. 여기서 rgba는 수정될 이미지입니다.
		Src:  image.NewUniform(col), // Src는 텍스트의 색상을 지정합니다. 여기서는 빨간색을 사용합니다.
		Face: basicfont.Face7x13,    // Face는 사용할 폰트를 지정합니다. 여기서는 기본 폰트 Face7x13을 사용합니다.
		Dot:  point,                 // Dot은 텍스트를 시작할 위치를 지정합니다.
	}
	// 지정된 설정을 사용하여 이미지에 label을 그립니다.
	d.DrawString(label)

	return rgba

}

func isImageFile(key string) (bool, string) {
	// 지원되는 이미지 파일 확장자 리스트
	supportedExtensions := []string{".jpg", ".jpeg", ".png"}
	for _, ext := range supportedExtensions {
		if strings.HasSuffix(strings.ToLower(key), ext) {
			return true, ext[1:]
		}
	}
	return false, ""
}

func handleImage(wg *sync.WaitGroup, client *s3.S3, record *events.S3EventRecord) {
	defer wg.Done()
	s3Entity := record.S3
	bucket := s3Entity.Bucket.Name
	key := s3Entity.Object.Key
	log.Println("start to process ", key)
	isImage, ext := isImageFile(key)

	if !isImage {
		return
	}

	// S3에서 이미지 가져오기
	resp, err := client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// 에러 처리
		log.Fatalf("Unable to download item %q, %v", key, err)
	}
	defer resp.Body.Close()

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		log.Fatalf("Unable to decode image %q, %v", key, err)
	}

	bounds := img.Bounds()
	var labeledImage image.Image
	if bounds.Max.X < 20 && bounds.Max.Y < 20 {
		labeledImage = addLabel(img, 0, 0, "This is watermark")
		log.Printf("add label to the image at point %v, %v\n", bounds.Max.X, bounds.Max.X)
	} else {
		labeledImage = addLabel(img, 20, 20, "This is watermark")
		log.Printf("add label to the image at point %v, %v\n", bounds.Max.X, bounds.Max.X)
	}

	buf := new(bytes.Buffer)

	switch ext {
	case "png":
		err := png.Encode(buf, labeledImage)
		if err != nil {
			log.Fatal(err)
		}
	case "jpeg", "jpg":
		err := jpeg.Encode(buf, labeledImage, nil)
		if err != nil {
			log.Fatal(err)
		}
	}
	log.Println("Saving image to s3")
	_, err = client.PutObject(&s3.PutObjectInput{
		Bucket:      aws.String(s3Entity.Bucket.Name),                             // S3 버킷 이름
		Key:         aws.String(fmt.Sprintf("labeled-images/%v", path.Base(key))), // 저장될 이미지의 키 (파일 이름)
		Body:        bytes.NewReader(buf.Bytes()),
		ContentType: aws.String(fmt.Sprintf("image/%v", ext)), // 또는 "image/png" 등
	})

	if err != nil {
		log.Fatalf("failed to save %q to s3: %v", key, err)
	}
}

func HandleRequest(ctx context.Context, s3Event events.S3Event) {
	log.Println("Handler start..")
	// AWS 세션 생성
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	client := s3.New(sess)

	var wg sync.WaitGroup
	// 이벤트로부터 버킷과 키 추출

	log.Println("Reading s3 records..")
	for _, record := range s3Event.Records {
		wg.Add(1)
		go handleImage(&wg, client, &record)
	}
	wg.Wait()
}

func main() {
	lambda.Start(HandleRequest)
}
