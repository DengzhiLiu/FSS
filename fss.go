package main

/*
#include <MagickWand/MagickWand.h>
*/
import "C"

import (
    "fmt"
    "io"
    "os"
    "log"
    "strings"
    "path"
    "unsafe"
    "net/http"
    "io/ioutil"
    "html/template"
    "runtime/debug"
)

const (
    ListDir      = 0x0001
    UPLOAD_DIR   = "./uploads"
    TRANSFORM_DIR = "./transform"
    TEMPLATE_DIR = "./views"
)

//templates := make(map[string]*template.Template)
//var templates map[string]*template.Template
var templates = make(map[string]*template.Template)


func init() {
    fileInfoArr, err := ioutil.ReadDir(TEMPLATE_DIR)
    check(err)
    var templateName, templatePath string
    for _, fileInfo := range fileInfoArr {
        templateName = fileInfo.Name()
        if ext := path.Ext(templateName); ext != ".html" {
            continue
        }
        templatePath = TEMPLATE_DIR + "/" + templateName 
        log.Println("Loading template:", templatePath) 
        t := template.Must(template.ParseFiles(templatePath))             
        templates[templateName] = t
    }
}

func check(err error) {
    if err != nil {
        panic(err)
    }
}

func renderHtml(w http.ResponseWriter, tmpl string, locals map[string]interface{}) {
    templateName := tmpl + ".html"
    err := templates[templateName].Execute(w, locals)
    check(err)
}

func isExists(path string) (bool, error) {
    _, err := os.Stat(path)
    if err == nil {
        return true, nil
    }

    /*
    if e, ok := err.(*os.PathError); ok && e.Error == os.ENOENT {
        return false, nil
    }
    */
    return false, err
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
    log.SetFlags(log.Lshortfile | log.LstdFlags)
    if r.Method == "GET" {
        renderHtml(w, "upload", nil);
    }
    if r.Method == "POST" {
        //log.Println(r.MultipartForm.File)
        f, h, err := r.FormFile("image")
        check(err)
        filename := h.Filename
        defer f.Close()
        log.Println("uploading ", filename)
        t, err := ioutil.TempFile(UPLOAD_DIR, filename)
        check(err)
        defer t.Close()
        _, err = io.Copy(t, f)
        check(err)
        _, file := path.Split(t.Name())
        http.Redirect(w, r, "/view?id="+file, http.StatusFound)
    }
}

func viewHandler(w http.ResponseWriter, r *http.Request) {
    imageId := r.FormValue("id")
    imagePath := UPLOAD_DIR + "/" + imageId
    exists, _ := isExists(imagePath)
    if !exists {
        http.NotFound(w, r)
        return
    }

    w.Header().Set("Content-Type", "image")
    http.ServeFile(w, r, imagePath)
}

func MogrHandler(imagename string, proc string, w http.ResponseWriter, r *http.Request) {

    imagePath := UPLOAD_DIR + "/" + imagename
    exists, _ := isExists(imagePath)
    if !exists {
        http.NotFound(w, r)
        return
    }

    ops := strings.SplitN(proc, "/", -1)
    if len(ops) % 2 != 0 {
        panic("error num operations")
    }

    operates := make(map[string]string)
    for i := 0; i < len(ops); i += 2 {
        operates[ops[i] ] = ops[i + 1]
    }

    var status C.MagickBooleanType
    var magick_wand *C.MagickWand;

    // read image
    C.MagickWandGenesis();
    magick_wand = C.NewMagickWand();
    cimagePath := C.CString(imagePath)
    defer C.free(unsafe.Pointer(cimagePath))
    status = C.MagickReadImage(magick_wand, cimagePath);
    if status == C.MagickFalse {
        panic("error when read image")
    }

    // Get the image's width and height
    //width := C.MagickGetImageWidth(magick_wand);
    //height := C.MagickGetImageHeight(magick_wand);
    _, ok :=  operates["thumbnail"]
    if ok {
        //fmt.Println("thumbnail", thumbnail)
        C.MagickResizeImage(magick_wand, 106, 80, C.LanczosFilter);
    }

    _, ok = operates["crop"]
    if ok {
        //fmt.Println("crop", crop)
        C.MagickCropImage(magick_wand, 802, 802, 10, 10);
    }

    _, ok = operates["quality"]
    if ok {
        //fmt.Println("quality", quality)
        C.MagickSetImageCompressionQuality(magick_wand, 90);
    }

    // Turn the images into a thumbnail sequence.
    /*
    C.MagickResetIterator(magick_wand);
    for {
        if C.MagickNextImage(magick_wand) != C.MagickFalse {
            break;
        }
        C.MagickResizeImage(magick_wand, 106, 80, C.LanczosFilter);
    }
    */
    // Set the compression quality to 95 (high quality = low compression)

    // Write the image then destroy it.
    outputPath := TRANSFORM_DIR + "/" + imagename
    coutputPath := C.CString(outputPath)
    defer C.free(unsafe.Pointer(coutputPath))
    status = C.MagickWriteImages(magick_wand, coutputPath, C.MagickTrue);
    if status == C.MagickFalse {
        panic("error when write image")
    }

    magick_wand = C.DestroyMagickWand(magick_wand);
    C.MagickWandTerminus();

    w.Header().Set("Content-Type", "image")
    http.ServeFile(w, r, outputPath)
    //procparams := strings.SplitN(param[2], "/", -1)
}

func procHandler(request string, w http.ResponseWriter, r *http.Request) {
    log.SetFlags(log.Lshortfile | log.LstdFlags)
    defer func() {
        if r := recover(); r != nil {
            log.Println("WARN: runtime error caught ", r)
        }
    }()

    params := strings.SplitN(request, "/", 3)
    if len(params) < 3 {
        panic("wrong proc request")
    }
    param := strings.SplitN(params[1], "?", 2)
    if len(param) < 2 {
        panic("wrong proc request")
    }

    imageId := param[0]
    proctype := param[1]
    //log.Println("filename %s protype %s params %s", imageId, proctype, params[2])

    switch  proctype {
    case "imageMogr2" :
        MogrHandler(imageId, params[2], w, r)
    default :
        panic("unknown manipulation")
    }
}

func listHandler(w http.ResponseWriter, r *http.Request) {
    uri := r.RequestURI
    if (uri == "/") {
        fileInfoArr, err := ioutil.ReadDir("./uploads")         
        check(err)
        locals := make(map[string]interface{})
        images := []string{}
        for _, fileInfo := range fileInfoArr {
            images = append(images, fileInfo.Name())
        }
        locals["images"] = images
        renderHtml(w, "list", locals)
    } else {
        procHandler(uri, w, r)
    }
}

func safeHandler(fn http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if e, ok := recover().(error); ok {
                http.Error(w, e.Error(), 
                    http.StatusInternalServerError)                      
                // 或者输出自定义的 50x 错误页面                     
                // w.WriteHeader(http.StatusInternalServerError)                     
                // renderHtml(w, "error", e)                      
                // logging
                log.Println("WARN: panic in %v. - %v", fn, e)
                log.Println(string(debug.Stack()))
            }
        }()
        fn(w, r)
    }
}

func staticDirHandler(mux *http.ServeMux, prefix string, staticDir string, flags int) {
    mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
        file := staticDir + r.URL.Path[len(prefix)-1:]             
        fmt.Println(file)
        if (flags & ListDir) == 0 {
            if exists, _ := isExists(file); !exists {
                http.NotFound(w, r)                     
                return
            }
        }
        http.ServeFile(w, r, file)
    })
}

func main() {
    mux := http.NewServeMux()
    staticDirHandler(mux, "/assets/", "./public", 0)
    mux.HandleFunc("/", safeHandler(listHandler))         
    mux.HandleFunc("/view", safeHandler(viewHandler))         
    mux.HandleFunc("/upload", safeHandler(uploadHandler))          
    err := http.ListenAndServe(":8080", mux)
    if err != nil {
        log.Fatal("ListenAndServe: ", err.Error())
    }
} 
