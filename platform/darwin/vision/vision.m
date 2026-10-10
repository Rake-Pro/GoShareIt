//go:build darwin && cgo

// Apple Vision text recognition (VNRecognizeTextRequest) for the GoShareIt
// editor. On-device only: Vision is a public system framework, no network,
// no entitlement, no TCC prompt.
//
// Memory: cgo compiles this file without ARC, so every object created with
// alloc/init is autoreleased into the function's @autoreleasepool and every
// CoreGraphics object is released explicitly.

#import "vision.h"
#import <Foundation/Foundation.h>
#import <Vision/Vision.h>
#import <CoreGraphics/CoreGraphics.h>

#include <stdlib.h>
#include <string.h>

#if __has_feature(objc_arc)
#define GSI_AUTORELEASE(x) (x)
#else
#define GSI_AUTORELEASE(x) [(x) autorelease]
#endif

static char g_last_error[512];

static void gsi_set_error(NSString *msg) {
    const char *c = msg ? msg.UTF8String : NULL;
    if (c == NULL) {
        g_last_error[0] = '\0';
        return;
    }
    strncpy(g_last_error, c, sizeof(g_last_error) - 1);
    g_last_error[sizeof(g_last_error) - 1] = '\0';
}

const char *gsi_vision_last_error(void) {
    return g_last_error;
}

static char *gsi_strdup(NSString *s) {
    const char *c = s ? s.UTF8String : NULL;
    return strdup(c ? c : "");
}

// gsi_flip converts a normalized, lower-left-origin Vision box to top-left
// image pixels.
static void gsi_flip(CGRect bb, int width, int height, float *x, float *y, float *w, float *h) {
    *x = (float)(bb.origin.x * width);
    *y = (float)((1.0 - bb.origin.y - bb.size.height) * height);
    *w = (float)(bb.size.width * width);
    *h = (float)(bb.size.height * height);
}

static NSArray<NSString *> *gsi_split_langs(const char *langs, int nlangs) {
    NSMutableArray<NSString *> *out = [NSMutableArray arrayWithCapacity:(NSUInteger)nlangs];
    const char *p = langs;
    for (int i = 0; i < nlangs && p != NULL && *p != '\0'; i++) {
        NSString *tag = [NSString stringWithUTF8String:p];
        if (tag != nil) {
            [out addObject:tag];
        }
        p += strlen(p) + 1;
    }
    return out;
}

static VNRecognizeTextRequest *gsi_new_request(void) {
    VNRecognizeTextRequest *req = GSI_AUTORELEASE([[VNRecognizeTextRequest alloc] init]);
    if (@available(macOS 13.0, *)) {
        req.revision = VNRecognizeTextRequestRevision3;
    }
    return req;
}

int gsi_vision_languages(char **out) {
    *out = NULL;
    gsi_set_error(nil);
    @autoreleasepool {
        if (@available(macOS 12.0, *)) {
            NSError *err = nil;
            NSArray<NSString *> *langs = [gsi_new_request() supportedRecognitionLanguagesAndReturnError:&err];
            if (langs == nil) {
                gsi_set_error(err ? err.localizedDescription : @"Vision reported no languages");
                return 0;
            }
            size_t total = 1;
            for (NSString *l in langs) {
                total += strlen(l.UTF8String ? l.UTF8String : "") + 1;
            }
            char *buf = calloc(total + 1, 1);
            if (buf == NULL) {
                gsi_set_error(@"out of memory");
                return 0;
            }
            char *p = buf;
            int n = 0;
            for (NSString *l in langs) {
                const char *c = l.UTF8String;
                if (c == NULL || *c == '\0') {
                    continue;
                }
                size_t len = strlen(c);
                memcpy(p, c, len);
                p += len + 1; // calloc left the NUL
                n++;
            }
            *out = buf;
            return n;
        }
        gsi_set_error(@"Text recognition needs macOS 12 or newer.");
        return 0;
    }
}

int gsi_vision_recognize(const unsigned char *rgba, int width, int height, int stride,
                         const char *langs, int nlangs, int fast, gsi_vision_result **out) {
    *out = NULL;
    gsi_set_error(nil);
    if (rgba == NULL || width <= 0 || height <= 0 || stride < width * 4) {
        gsi_set_error(@"empty image");
        return 1;
    }
    @autoreleasepool {
        // Copy the pixels into a CFData the image owns, so nothing in Vision
        // or CoreGraphics can outlive the caller's buffer.
        CFDataRef data = CFDataCreate(NULL, rgba, (CFIndex)((size_t)stride * (size_t)height));
        if (data == NULL) {
            gsi_set_error(@"out of memory");
            return 2;
        }
        CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
        CGDataProviderRef dp = CGDataProviderCreateWithCFData(data);
        CFRelease(data);
        CGImageRef img = CGImageCreate((size_t)width, (size_t)height, 8, 32, (size_t)stride, cs,
                                       (CGBitmapInfo)kCGImageAlphaPremultipliedLast | kCGBitmapByteOrderDefault,
                                       dp, NULL, false, kCGRenderingIntentDefault);
        CGDataProviderRelease(dp);
        CGColorSpaceRelease(cs);
        if (img == NULL) {
            gsi_set_error(@"could not create the image for Vision");
            return 2;
        }

        VNRecognizeTextRequest *req = gsi_new_request();
        if (@available(macOS 13.0, *)) {
            req.automaticallyDetectsLanguage = (nlangs == 0);
        }
        req.recognitionLevel = fast ? VNRequestTextRecognitionLevelFast : VNRequestTextRecognitionLevelAccurate;
        req.usesLanguageCorrection = YES;
        if (nlangs > 0) {
            req.recognitionLanguages = gsi_split_langs(langs, nlangs);
        }

        VNImageRequestHandler *h = GSI_AUTORELEASE([[VNImageRequestHandler alloc] initWithCGImage:img options:@{}]);
        NSError *err = nil;
        BOOL ok = [h performRequests:@[ req ] error:&err];
        CGImageRelease(img);
        if (!ok) {
            gsi_set_error(err ? err.localizedDescription : @"text recognition failed");
            return 3;
        }

        NSArray *obs = req.results;
        gsi_vision_result *res = calloc(1, sizeof(gsi_vision_result));
        if (res == NULL) {
            gsi_set_error(@"out of memory");
            return 4;
        }
        if (obs.count > 0) {
            res->lines = calloc(obs.count, sizeof(gsi_vision_line));
            if (res->lines == NULL) {
                free(res);
                gsi_set_error(@"out of memory");
                return 4;
            }
        }
        NSCharacterSet *ws = [NSCharacterSet whitespaceAndNewlineCharacterSet];
        for (VNRecognizedTextObservation *o in obs) {
            VNRecognizedText *top = [[o topCandidates:1] firstObject];
            if (top == nil) {
                continue;
            }
            gsi_vision_line *line = &res->lines[res->line_count++];
            NSString *s = top.string;
            line->text = gsi_strdup(s);
            line->conf = top.confidence;
            gsi_flip(o.boundingBox, width, height, &line->x, &line->y, &line->w, &line->h);

            NSUInteger len = s.length;
            if (len == 0) {
                continue;
            }
            line->words = calloc(len, sizeof(gsi_vision_word)); // at most one word per character
            if (line->words == NULL) {
                continue;
            }
            NSUInteger pos = 0;
            while (pos < len) {
                while (pos < len && [ws characterIsMember:[s characterAtIndex:pos]]) {
                    pos++;
                }
                if (pos >= len) {
                    break;
                }
                NSUInteger start = pos;
                while (pos < len && ![ws characterIsMember:[s characterAtIndex:pos]]) {
                    pos++;
                }
                NSRange r = NSMakeRange(start, pos - start);
                gsi_vision_word *w = &line->words[line->word_count++];
                w->text = gsi_strdup([s substringWithRange:r]);
                w->conf = top.confidence;
                NSError *rerr = nil;
                VNRectangleObservation *box = [top boundingBoxForRange:r error:&rerr];
                CGRect bb = box != nil ? box.boundingBox : o.boundingBox;
                gsi_flip(bb, width, height, &w->x, &w->y, &w->w, &w->h);
            }
        }
        *out = res;
        return 0;
    }
}

void gsi_vision_free(gsi_vision_result *r) {
    if (r == NULL) {
        return;
    }
    for (int i = 0; i < r->line_count; i++) {
        gsi_vision_line *l = &r->lines[i];
        for (int j = 0; j < l->word_count; j++) {
            free(l->words[j].text);
        }
        free(l->words);
        free(l->text);
    }
    free(r->lines);
    free(r);
}
