package main

import (
    "bytes"
    "image/png"
    "testing"
)
func TestSplashEmbedsRealBrandMark(t *testing.T) {
    if !verifySplashLogic() {t.Fatal("splash icon missing")}
    im,err:=png.Decode(bytes.NewReader(brandImage))
    if err!=nil {t.Fatal(err)}
    if im.Bounds().Dx()!=256||im.Bounds().Dy()!=256 {t.Fatalf("wrong logo dimensions: %v",im.Bounds())}
}
// No modal prompt in automated tests; this verifies the bitmap can be loaded
// by Windows without writing anything to disk.
func TestOriginalBrandBitmapCreation(t *testing.T) {
    bitmap:=makeBrandBitmap()
    if bitmap==0 {t.Fatal("GDI logo bitmap not created")}
    pDeleteObject.Call(bitmap)
}
