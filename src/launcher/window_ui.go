package main

// Native Windows splash screen for the Rev Visión launcher. The application
// UI is never changed, and this screen does not access patient data.
import (
    "bytes"
    _ "embed"
    "fmt"
    "image/png"
    "runtime"
    "syscall"
    "time"
    "unsafe"
)

//go:embed brand.png
var brandImage []byte

const (
    wsChild = 0x40000000
    wsVisible = 0x10000000
    wsCaption = 0x00C00000
    wsBorder = 0x00800000
    wmClose = 0x0010
    wmDestroy = 0x0002
    wmCtlColorStatic = 0x0138
    wmSetFont = 0x0030
    stmSetImage = 0x0172
    imageBitmap = 0
    rgbPurple = 0x00852b89
)
var (
    user32 = syscall.NewLazyDLL("user32.dll")
    gdi32 = syscall.NewLazyDLL("gdi32.dll")
    kernel32 = syscall.NewLazyDLL("kernel32.dll")
    launcherModule uintptr
    pCreateWindow = user32.NewProc("CreateWindowExW")
    pSendMessage = user32.NewProc("SendMessageW")
    pDefWindowProc = user32.NewProc("DefWindowProcW")
    pDestroyWindow = user32.NewProc("DestroyWindow")
    pPostQuitMessage = user32.NewProc("PostQuitMessage")
    pSetWindowText = user32.NewProc("SetWindowTextW")
    pMoveWindow = user32.NewProc("MoveWindow")
    pCreateBrush = gdi32.NewProc("CreateSolidBrush")
    pDeleteObject = gdi32.NewProc("DeleteObject")
    pSetBkMode = gdi32.NewProc("SetBkMode")
    pSetTextColor = gdi32.NewProc("SetTextColor")
    whiteBrush uintptr
    purpleBrush uintptr
    globalWindowProc = syscall.NewCallback(onSplashMessage)
)

type windowClass struct {
    Style uint32
    WndProc uintptr
    ClsExtra int32
    WndExtra int32
    Instance uintptr
    Icon uintptr
    Cursor uintptr
    Background uintptr
    MenuName *uint16
    ClassName *uint16
}
type winMessage struct {
    Hwnd uintptr
    Message uint32
    WParam uintptr
    LParam uintptr
    Time uint32
    PointX int32
    PointY int32
    Private uint32
}
type dibHeader struct {
    Size uint32
    Width int32
    Height int32
    Planes uint16
    BitCount uint16
    Compression uint32
    SizeImage uint32
    Xpels int32
    Ypels int32
    Used uint32
    Important uint32
}
type dibInfo struct { Header dibHeader; Colors [1]uint32 }

type launchSplash struct {
    hwnd uintptr
    status uintptr
    percent uintptr
    bar uintptr
    bitmap uintptr
    fonts []uintptr
    ready chan struct{}
    isReady bool
    lastPercent int
}
func wide(s string) *uint16 { v,_:=syscall.UTF16PtrFromString(s);return v }
func callWindow(class, text string, style uint32, x,y,w,h int32, parent uintptr) uintptr {
    hwnd,_,_:=pCreateWindow.Call(0,uintptr(unsafe.Pointer(wide(class))),uintptr(unsafe.Pointer(wide(text))),uintptr(style),uintptr(x),uintptr(y),uintptr(w),uintptr(h),parent,0,launcherModule,0)
    return hwnd
}
func font(height int32, weight int32) uintptr {
    p:=gdi32.NewProc("CreateFontW")
    h,_,_:=p.Call(uintptr(height),0,0,0,uintptr(weight),0,0,0,1,0,0,5,0,uintptr(unsafe.Pointer(wide("Segoe UI"))))
    return h
}
func useFont(hwnd, value uintptr) { if hwnd!=0 && value!=0 {pSendMessage.Call(hwnd,wmSetFont,value,1)} }
func onSplashMessage(hwnd uintptr, message uint32, wp,lp uintptr) uintptr {
    switch message {
    case wmCtlColorStatic:
        pSetBkMode.Call(wp,1)
        if lp!=0 {
            pSetTextColor.Call(wp,rgbPurple)
        }
        if purpleBrush!=0 && lp!=0 && lp==currentSplashBar() {return purpleBrush}
        return whiteBrush
    case wmClose:
        pDestroyWindow.Call(hwnd);return 0
    case wmDestroy:
        pPostQuitMessage.Call(0);return 0
    }
    r,_,_:=pDefWindowProc.Call(hwnd,uintptr(message),wp,lp)
    return r
}
var splashBar uintptr
func currentSplashBar() uintptr {return splashBar}

// Convert the original, transparent brand mark to an in-memory Win32 bitmap.
// No temporary images or user-data files are written to disk.
func makeBrandBitmap() uintptr {
    src,err:=png.Decode(bytes.NewReader(brandImage));if err!=nil{return 0}
    const w,h=70,70
    info:=dibInfo{Header:dibHeader{Size:40,Width:w,Height:-h,Planes:1,BitCount:32}}
    var pixels uintptr
    p:=gdi32.NewProc("CreateDIBSection")
    bmp,_,_:=p.Call(0,uintptr(unsafe.Pointer(&info)),0,uintptr(unsafe.Pointer(&pixels)),0,0)
    if bmp==0||pixels==0 {return 0}
    buf:=unsafe.Slice((*byte)(unsafe.Pointer(pixels)),w*h*4)
    bounds:=src.Bounds()
    for y:=0;y<h;y++ { for x:=0;x<w;x++ {
        sx:=bounds.Min.X+x*bounds.Dx()/w;sy:=bounds.Min.Y+y*bounds.Dy()/h
        r,g,b,a:=src.At(sx,sy).RGBA()
        i:=(y*w+x)*4
        buf[i]=byte((b+65535-a)>>8)
        buf[i+1]=byte((g+65535-a)>>8)
        buf[i+2]=byte((r+65535-a)>>8)
        buf[i+3]=0
    }}
    return bmp
}
func startSplash() *launchSplash {
    s:=&launchSplash{ready:make(chan struct{}),lastPercent:-1}
    go s.loop()
    select {
    case <-s.ready:
    case <-time.After(2*time.Second):
    }
    return s
}
func (s *launchSplash) loop(){
    runtime.LockOSThread()
    defer runtime.UnlockOSThread()
    whiteBrush,_,_=pCreateBrush.Call(0x00FFFFFF)
    purpleBrush,_,_=pCreateBrush.Call(rgbPurple)
    defer func(){
        if s.bitmap!=0 {pDeleteObject.Call(s.bitmap)}
        for _,x:=range s.fonts {if x!=0 {pDeleteObject.Call(x)}}
        if whiteBrush!=0 {pDeleteObject.Call(whiteBrush)}
        if purpleBrush!=0 {pDeleteObject.Call(purpleBrush)}
    }()
    inst,_,_:=kernel32.NewProc("GetModuleHandleW").Call(0)
    launcherModule=inst
    className:=wide("RevVisionLauncherSplashV1")
    wc:=windowClass{WndProc:globalWindowProc,Instance:inst,Background:whiteBrush,ClassName:className}
    user32.NewProc("RegisterClassW").Call(uintptr(unsafe.Pointer(&wc)))
    sw,_,_:=user32.NewProc("GetSystemMetrics").Call(0)
    sh,_,_:=user32.NewProc("GetSystemMetrics").Call(1)
    s.hwnd=callWindow("RevVisionLauncherSplashV1","Rev Visión · Actualizaciones",wsCaption|wsVisible,int32(sw)/2-240,int32(sh)/2-130,480,260,0)
    if s.hwnd==0 {close(s.ready);return}
    s.bitmap=makeBrandBitmap()
    if s.bitmap!=0 {
        logo:=callWindow("STATIC","",wsChild|wsVisible|0x000E,20,27,72,72,s.hwnd)
        if logo!=0 {pSendMessage.Call(logo,stmSetImage,imageBitmap,s.bitmap)}
    }
    fTitle:=font(-26,700);fOther:=font(-15,400);fSmall:=font(-13,400)
    s.fonts=[]uintptr{fTitle,fOther,fSmall}
    heading:=callWindow("STATIC","Rev Visión",wsChild|wsVisible,106,27,340,38,s.hwnd)
    useFont(heading,fTitle)
    sub:=callWindow("STATIC","Óptica y Salud Visual",wsChild|wsVisible,108,66,340,22,s.hwnd)
    useFont(sub,fOther)
    s.status=callWindow("STATIC","Buscando actualizaciones…",wsChild|wsVisible,23,123,427,30,s.hwnd)
    useFont(s.status,fOther)
    background:=callWindow("STATIC","",wsChild|wsVisible|wsBorder,23,167,430,13,s.hwnd)
    _=background
    s.bar=callWindow("STATIC","",wsChild|wsVisible,24,168,1,11,s.hwnd)
    splashBar=s.bar
    s.percent=callWindow("STATIC","Preparando el programa…",wsChild|wsVisible,23,189,427,22,s.hwnd)
    useFont(s.percent,fSmall)
    user32.NewProc("ShowWindow").Call(s.hwnd,5)
    user32.NewProc("UpdateWindow").Call(s.hwnd)
    s.isReady=true
    close(s.ready)
    var msg winMessage
    getMessage:=user32.NewProc("GetMessageW")
    for {
        result,_,_:=getMessage.Call(uintptr(unsafe.Pointer(&msg)),0,0,0)
        if int32(result)<=0 {break}
        user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
        user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
    }
}
func (s *launchSplash) statusText(msg string){
    if s==nil||!s.isReady||s.status==0{return}
    pSetWindowText.Call(s.status,uintptr(unsafe.Pointer(wide(msg))))
}
func (s *launchSplash) progress(done,total int64){
    if s==nil||!s.isReady||total<=0{return}
    pct:=int(done*100/total)
    if pct<0 {pct=0};if pct>100 {pct=100}
    if pct==s.lastPercent{return}
    s.lastPercent=pct
    if s.bar!=0 {pMoveWindow.Call(s.bar,24,168,uintptr(max(1,pct*428/100)),11,1)}
    if s.percent!=0 {pSetWindowText.Call(s.percent,uintptr(unsafe.Pointer(wide(fmt.Sprintf("Descargando actualización · %d %%",pct)))))}
}
func (s *launchSplash) complete(msg string){
    if s!=nil {s.statusText(msg);s.progress(100,100)}
}
func (s *launchSplash) close(){
    if s!=nil && s.hwnd!=0 {
        user32.NewProc("PostMessageW").Call(s.hwnd,wmClose,0,0)
    }
}
func (s *launchSplash) parent() uintptr {if s==nil{return 0};return s.hwnd}

func askToInstall(s *launchSplash, localVersion, newVersion string) bool {
    // Explicit consent: never download or replace the installed version after
    // "No"/close. First installation is handled separately by repairIfRequired.
    message:=fmt.Sprintf("Hay una actualización disponible.\n\nVersión instalada: %s\nVersión nueva: %s\n\n¿Deseas instalarla ahora?\n\nSí = Instalar ahora\nNo = Más tarde (abrir la versión actual)",localVersion,newVersion)
    title:="Rev Visión · Actualización disponible"
    result,_,_:=user32.NewProc("MessageBoxW").Call(s.parent(),uintptr(unsafe.Pointer(wide(message))),uintptr(unsafe.Pointer(wide(title))),0x00000004|0x00000020|0x00000100)
    return result==6 // IDYES
}

func verifySplashLogic() bool {return len(brandImage)>0 && rgbPurple!=0}
