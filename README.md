# Bubsie Api

Mobil AI uygulamaları (iOS/Android) icin merkezi proxy, guvenlik ve yonetim katmani.
Bu dosya mobil uygulama gelistirirken referans belgesi olarak kullanilir.

---

## Mimari

```
iOS/Android App
    │
    │  Firebase Auth ile giris yap
    │  idToken al
    │
    ▼
Bubsie Api (Go / Fiber v2)  ──────────────────────────────────────────
    │                                                                │
    ├─ [Auth Middleware]                                              │
    │   ├─ Mobil API: Firebase Admin SDK (service account)           │
    │   │   ├─ Ban kontrolu (isUserBanned)                           │
    │   │   └─ X-Install-Seed ile tek seferlik baslangic kredi claim │
    │   └─ Admin Panel: Lightweight JWT (Google public keys)         │
    │       └─ Admin email kontrolu + Ban kontrolu                   │
    │                                                                │
    ├─ [Rate Limiter]                                                │
    │   └─ Kullanici bazli (UID) veya IP bazli                      │
    │                                                                │
    ├─ [AI Proxy Layer]  ───► fal.ai                                 │
    │                    ───► Replicate                              │
    │                    ───► DeepSeek                               │
    │                    ───► OpenRouter                             │
    │                    ───► Gemini                                 │
    │                                                                │
    ├─ [Cloudflare R2]                                               │
    │   ├─ uploads/      (kullanici yuklenen gorseller)              │
    │   ├─ results/      (AI sonuc gorselleri, kalici)               │
    │   └─ SSRF-safe HTTP client (DNS rebinding koruması)            │
    │                                                                │
    ├─ [SQLite DB]                                                   │
    │   ├─ users           (Firebase UID, email, kredi, ban)         │
    │   ├─ request_logs    (provider, model, prompt, sure, durum)    │
    │   ├─ provider_settings (aktif/pasif, model listeleri)          │
    │   ├─ categories      (photo/video, app bazli)                  │
    │   ├─ templates       (prompt, provider, before/after demo)     │
    │   ├─ slider_items    (banner, tarih araligi)                   │
    │   ├─ quick_buttons   (slider alti kisayollar)                  │
    │   ├─ onboarding_media (video/gorsel)                           │
    │   ├─ onboarding_reviews (kullanici yorumlari)                  │
    │   ├─ translations    (16 dil, AI destekli ceviri)              │
    │   ├─ device_tokens   (FCM push token'lari)                     │
    │   └─ install_credit_claims (cihaz bazli ilk kredi kilidi)      │
    │                                                                │
    ├─ [Ceviri Servisi]                                              │
    │   └─ DeepSeek AI ile 16 dile otomatik ceviri                  │
    │                                                                │
    ├─ [RevenueCat]                                                  │
    │   └─ Gelir istatistikleri ve abonelik durumu                   │
    │                                                                │
    ├─ [Push Notifications]  (Firebase Cloud Messaging)              │
    │   ├─ device_tokens   (iOS/Android FCM token'lari, uid bazli)   │
    │   └─ Admin broadcast (baslik/govde/deep-link ile manuel)       │
    │                                                                │
    └─ [Admin Panel]  /panel                                         │
        ├─ Dashboard     (kullanici sayisi, gelir, istek istatistik) │
        ├─ Providers     (aktif/pasif, model listeleri, health check)│
        ├─ CMS           (kategori, template, slider, onboarding)    │
        ├─ Playground    (AI test arayuzu)                           │
        ├─ Loglar        (son istekler tablosu, sayfalanmis)         │
        ├─ Kullanicilar  (kredi yonetimi, ban)                       │
        └─ Notifications (manuel push gonder — iOS/Android/Hepsi)    │
─────────────────────────────────────────────────────────────────────
```

### Neden Bu Mimari?

- **Guvenlik:** API key'ler sunucuda kalir. Uygulama decompile edilse bile sadece backend URL'i gorunur.
- **Esneklik:** Provider degistirmek icin mobil uygulamaya guncelleme atmaya gerek yok — backend'de tek satir degisir.
- **Olceklenebilirlik:** Go'nun concurrency modeli ayni anda yuezlerce istek isleyebilir.
- **Coklu Uygulama:** Tek bir backend, birden fazla mobil uygulamayi destekler (`app_id` bazli icerik ayrimi).

---

## Teknik Stack

| Bilesen          | Teknoloji                           |
|------------------|-------------------------------------|
| Dil              | Go 1.22+                           |
| Framework        | Fiber v2                            |
| Auth (Mobil)     | Firebase Admin SDK                  |
| Push Bildirim    | Firebase Cloud Messaging (APNs via FCM) |
| Auth (Panel)     | Lightweight JWT (Google certs)      |
| Veritabani       | SQLite (GORM)                       |
| Depolama         | Cloudflare R2 (S3 uyumlu)          |
| AI Provider'lar  | fal.ai, Replicate, DeepSeek, OpenRouter, Gemini |
| Ceviri           | DeepSeek AI (16 dil)               |
| Gelir Izleme     | RevenueCat API                      |
| Admin Panel      | Gomulu HTML (Tailwind + Alpine.js)  |
| Deploy           | Docker / Coolify                    |

---

## Proje Yapisi

```
bubsiebackend/
├── cmd/server/
│   └── main.go                    # Entry point, Fiber config, graceful shutdown
├── internal/
│   ├── config/
│   │   └── config.go              # .env yukleme, tum yapilandirma
│   ├── database/
│   │   └── database.go            # SQLite baglantisi, GORM AutoMigrate, index migration
│   ├── handler/
│   │   ├── admin.go               # Admin endpoint'leri (stats, providers, users, logs)
│   │   ├── content.go             # CMS CRUD (kategori, template, slider, onboarding, review, ceviri)
│   │   ├── health.go              # GET /health
│   │   ├── notification.go        # FCM device token kayit + admin push broadcast
│   │   ├── playground.go          # Admin AI playground (test transform, meta)
│   │   ├── transform.go           # POST /api/v1/transform (AI proxy)
│   │   └── user.go                # /me, /providers, /history, /upload
│   ├── middleware/
│   │   ├── admin.go               # Admin email kontrolu
│   │   ├── auth.go                # Firebase Auth + Lightweight JWT + ban check
│   │   └── ratelimit.go           # UID/IP bazli rate limiting
│   ├── model/
│   │   ├── request.go             # Request DTO'lari
│   │   ├── response.go            # Standart API yanit formati
│   │   └── user.go                # GORM DB modelleri (12 tablo)
│   ├── router/
│   │   └── router.go              # Tum route tanimlari, middleware zinciri
│   ├── service/
│   │   ├── firebase.go            # Firebase Admin SDK (opsiyonel)
│   │   ├── revenuecat.go          # RevenueCat gelir API'si
│   │   ├── translate.go           # DeepSeek AI ile coklu dil ceviri
│   │   ├── provider/
│   │   │   ├── provider.go        # Provider interface, Registry pattern
│   │   │   ├── fal.go             # fal.ai proxy + health check
│   │   │   ├── replicate.go       # Replicate proxy + health check
│   │   │   ├── deepseek.go        # DeepSeek proxy + health check
│   │   │   ├── openrouter.go      # OpenRouter proxy + health check
│   │   │   └── gemini.go          # Google Gemini proxy + health check
│   │   └── storage/
│   │       └── r2.go              # Cloudflare R2 (S3) upload/download, SSRF koruması
│   └── web/
│       ├── web.go                 # embed.FS ile panel serve
│       └── templates/
│           └── index.html         # Admin panel SPA (SRI-korumalı CDN'ler)
├── Dockerfile                     # Multi-stage build (Alpine, CGO)
├── docker-compose.yml             # Local gelistirme
├── .env.example                   # Ornek yapilandirma
└── go.mod
```

---

## API Endpoints

### Genel (Auth gerektirmez)

| Method | Path                    | Aciklama                              |
|--------|-------------------------|---------------------------------------|
| GET    | `/`                     | /panel'e yonlendirir                  |
| GET    | `/health`               | Sunucu durum kontrolu                 |
| GET    | `/panel`                | Admin dashboard (HTML)                |
| GET    | `/api/config/firebase`  | Firebase web SDK yapilandirmasi       |

### Mobil Uygulama API (Firebase Auth + X-Install-Seed gerekli)

| Method | Path                    | Aciklama                                                   |
|--------|-------------------------|------------------------------------------------------------|
| GET    | `/api/v1/me`            | Kullanici profili, kredi, pro durum, kota                  |
| GET    | `/api/v1/providers`     | Aktif provider listesi (app icin)                          |
| POST   | `/api/v1/upload`        | Gorsel yukleme (multipart/form-data)                       |
| POST   | `/api/v1/transform`     | Gorsel donusturme istegi (AI proxy)                        |
| GET    | `/api/v1/history`       | Kullanicinin gecmis transform'lari                         |
| GET    | `/api/v1/categories`    | Kategoriler (?app_id=xx&lang=en)                           |
| GET    | `/api/v1/templates`     | Template listesi (?app_id=xx&category_id=1&featured=true&lang=en)|
| GET    | `/api/v1/slider`        | Slider/Banner listesi (?app_id=xx&lang=en)                 |
| GET    | `/api/v1/quick-buttons` | Slider altı 4 buton (type=photo\|video)                    |
| GET    | `/api/v1/onboarding`    | Onboarding medyalari (?app_id=xx&lang=en)                  |
| GET    | `/api/v1/reviews`       | Onboarding kullanici yorumlari (?lang=en)                  |
| GET    | `/api/v1/languages`     | Desteklenen dil listesi                                    |
| POST   | `/api/v1/device-token`  | FCM push token'ini kaydet/guncelle                        |
| DELETE | `/api/v1/device-token`  | Cihaz token'ini sil (logout icin)                         |

> Tum `/api/v1/*` endpoint'lerinde su iki header zorunludur: `Authorization: Bearer <firebase_id_token>` ve `X-Install-Seed: <stable_install_seed>`.

### Admin API (Firebase Auth + Admin Email gerekli)

| Method | Path                                 | Aciklama                        |
|--------|--------------------------------------|---------------------------------|
| GET    | `/api/admin/check`                   | Admin yetkisi dogrulama         |
| GET    | `/api/admin/stats`                   | Dashboard istatistikleri        |
| GET    | `/api/admin/stats/revenue`           | RevenueCat gelir verileri       |
| GET    | `/api/admin/stats/revenue-detailed`  | Detayli gelir analizi           |
| GET    | `/api/admin/users/count`             | Kullanici sayisi                |
| GET    | `/api/admin/providers`               | Provider listesi + durum        |
| GET    | `/api/admin/providers/health-check`  | Tum provider'lari test et       |
| POST   | `/api/admin/providers/test`          | Tek provider key testi          |
| POST   | `/api/admin/providers/toggle`        | Provider aktif/pasif toggle     |
| POST   | `/api/admin/providers/update-keys`   | Provider API key guncelle       |
| GET    | `/api/admin/logs`                    | Istek loglari (sayfalanmis)     |
| GET    | `/api/admin/users`                   | Kullanici listesi (kredi, pro)  |
| PUT    | `/api/admin/users/:id`               | Kredi/pro guncelle              |
| GET    | `/api/admin/categories`              | Kategori listesi (?type=photo\|video) |
| POST   | `/api/admin/categories`              | Yeni kategori olustur           |
| PUT    | `/api/admin/categories/:id`          | Kategori guncelle               |
| DELETE | `/api/admin/categories/:id`          | Kategori sil                    |
| GET    | `/api/admin/templates`               | Template listesi                |
| POST   | `/api/admin/templates`               | Yeni template olustur           |
| PUT    | `/api/admin/templates/:id`           | Template guncelle               |
| DELETE | `/api/admin/templates/:id`           | Template sil                    |
| GET    | `/api/admin/slider`                  | Slider listesi                  |
| POST   | `/api/admin/slider`                  | Yeni slider olustur             |
| PUT    | `/api/admin/slider/:id`              | Slider guncelle                 |
| DELETE | `/api/admin/slider/:id`              | Slider sil                      |
| GET    | `/api/admin/quick-buttons`           | Quick button listesi            |
| POST   | `/api/admin/quick-buttons`           | Quick button olustur            |
| PUT    | `/api/admin/quick-buttons/:id`       | Quick button guncelle           |
| DELETE | `/api/admin/quick-buttons/:id`       | Quick button sil                |
| GET    | `/api/admin/onboarding`              | Onboarding medya listesi        |
| POST   | `/api/admin/onboarding`              | Yeni onboarding ekle            |
| PUT    | `/api/admin/onboarding/:id`          | Onboarding guncelle             |
| DELETE | `/api/admin/onboarding/:id`          | Onboarding sil                  |
| GET    | `/api/admin/reviews`                 | Yorum listesi                   |
| POST   | `/api/admin/reviews`                 | Yeni yorum ekle                 |
| PUT    | `/api/admin/reviews/:id`             | Yorum guncelle                  |
| DELETE | `/api/admin/reviews/:id`             | Yorum sil                       |
| POST   | `/api/admin/translate`               | Icerik cevir (tum diller)       |
| GET    | `/api/admin/translations`            | Ceviri listesi                  |
| POST   | `/api/admin/upload-media`            | Medya dosyasi yukle (R2)        |
| POST   | `/api/admin/playground`              | AI test istegi                  |
| GET    | `/api/admin/playground/meta`         | Playground meta (action types, providers) |
| GET    | `/api/admin/notifications/stats`     | Kayitli cihaz token sayilari (ios/android) |
| POST   | `/api/admin/notifications/send`      | Manuel push notification gonder |

---

## API Yanit Formati

Tum endpoint'ler ayni JSON formatini doner:

```json
{
  "success": true,
  "data": { ... },
  "error": ""
}
```

Hata durumunda:

```json
{
  "success": false,
  "data": null,
  "error": "aciklayici hata mesaji"
}
```

> **Guvenlik notu:** 5xx hatalarinda detay gosterilmez, sunucu tarafinda loglanir.
> Provider hatalari istemciye genel mesajla doner (`"AI provider error — please try again"`).

---

## Mobil Uygulama Entegrasyonu (iOS/Android)

### 1. Auth Akisi

```
1. Firebase Auth ile giris yap (Google, Apple, Email)
2. idToken al:  user.getIDToken()
3. Cihazda kalici bir install seed uret ve sakla (Keychain/Keystore)
4. Her API isteginde header ekle:
   Authorization: Bearer <idToken>
   X-Install-Seed: <stable_install_seed>
```

> Ayni `X-Install-Seed` ile baslangic kredisi yalnizca bir kez verilir; uygulamayi silip tekrar yuklemek krediyi sifirlamaz.

> **Uyumluluk notu (iOS):**
> - Mevcut kullanicilar (DB'de `users` kaydi olanlar) mevcut app surumuyle calismaya devam eder.
> - Yeni kullanici/ilk kayit akisinda `X-Install-Seed` yoksa backend `400` doner (`missing X-Install-Seed header`).
> - `INITIAL_CREDITS=0` yapilabilir, ancak bu durumda reinstall-abuse korumasi anlamsizlasir.

### 2. Gorsel Yukleme (Transform oncesi)

```
POST /api/v1/upload
Headers:
  Authorization: Bearer <firebase_id_token>
  X-Install-Seed: <stable_install_seed>
  Content-Type: multipart/form-data

Form Data:
  image: <dosya>   (jpg, jpeg, png, webp, heic — max 10MB)
```

Yanit:
```json
{
  "success": true,
  "data": {
    "url": "https://cdn.kepa.app/uploads/uid123/20260413_143022_abc123.jpg",
    "filename": "20260413_143022_abc123.jpg",
    "size": 2458624
  }
}
```

### 3. Transform Istegi

```
POST /api/v1/transform
Headers:
  Authorization: Bearer <firebase_id_token>
  X-Install-Seed: <stable_install_seed>
  Content-Type: application/json

Body (standart):
{
  "provider": "fal.ai",
  "model": "fal-ai/flux/dev/image-to-image",
  "image_url": "https://...",
  "prompt": "Make this a watercolor painting",
  "params": {
    "strength": 0.8,
    "num_inference_steps": 30
  }
}

Body (family photo — anne/bebek/baba fotograflari ile):
{
  "provider": "fal.ai",
  "model": "fal-ai/flux-pulid",
  "image_url": "https://...",          // ana gorsel (e anne/bebek/baba fotografi gerekiyorsa yuklenir)
  "prompt": "a professional photo of the family...",
  "mom_image_url": "https://...",      // sadece template require_mom_photo=true ise
  "baby_image_url": "https://...",     // sadece template require_baby_photo=true ise
  "dad_image_url": "https://...",      // sadece template require_dad_photo=true ise
  "params": {
    "aspect_ratio": "1:1"
  }
}
```

> **Not:** `mom_image_url`, `baby_image_url`, `dad_image_url` alanlari opsiyoneldir. Sadece ilgili template'in `require_mom_photo`, `require_baby_photo` veya `require_dad_photo` alanlari `true` ise gonderilir. Backend bu URL'leri dogrudan fal.ai payload'ina iletir.

### 4. Transform Yaniti

```json
{
  "success": true,
  "data": {
    "result_url": "https://cdn.kepa.app/results/uid123/20260413_143022_abc123.jpg",
    "provider": "fal.ai",
    "model": "fal-ai/flux/dev/image-to-image",
    "metadata": { ... }
  }
}
```

> **Not:** `result_url` artik Cloudflare R2 CDN uzerinden kalici bir URL'dir.
> Provider'in gecici URL'i backend tarafindan indirilip R2'ye yuklenir.
> R2 yapilandirilmamissa, provider'in orijinal (gecici) URL'i doner.

### 5. Kullanici Profili

```
GET /api/v1/me
Headers:
  Authorization: Bearer <firebase_id_token>
  X-Install-Seed: <stable_install_seed>
```

Yanit:
```json
{
  "success": true,
  "data": {
    "uid": "abc123",
    "email": "user@example.com",
    "name": "John Doe",
    "photo": "https://...",
    "usage": {
      "today_total": 3,
      "today_success": 2,
      "all_time": 47
    },
    "rate_limit": {
      "max_per_window": 30,
      "window_seconds": 60
    },
    "member_since": "2026-04-01T10:00:00Z"
  }
}
```

### 6. Aktif Provider Listesi (App icin)

```
GET /api/v1/providers
Headers:
  Authorization: Bearer <firebase_id_token>
  X-Install-Seed: <stable_install_seed>
```

Yanit:
```json
{
  "success": true,
  "data": {
    "providers": [
      { "name": "fal.ai", "active": true },
      { "name": "replicate", "active": false },
      { "name": "deepseek", "active": true },
      { "name": "openrouter", "active": false },
      { "name": "gemini", "active": true }
    ]
  }
}
```

App sadece `active: true` olan provider'lari kullaniciya gostermelidir.

### 7. Transform Gecmisi

```
GET /api/v1/history?page=1&limit=20
Headers:
  Authorization: Bearer <firebase_id_token>
  X-Install-Seed: <stable_install_seed>
```

Yanit:
```json
{
  "success": true,
  "data": {
    "history": [
      {
        "id": 42,
        "provider": "fal.ai",
        "model": "fal-ai/flux/dev/image-to-image",
        "prompt": "watercolor painting",
        "image_url": "https://.../original.jpg",
        "result_url": "https://.../result.jpg",
        "status": "success",
        "duration_ms": 12340,
        "created_at": "2026-04-13T14:30:22Z"
      }
    ],
    "total": 47,
    "page": 1,
    "limit": 20
  }
}
```

### 8. Hata Durumlari

| HTTP Kodu | Anlami                                | Ne Yapilmali                     |
|-----------|---------------------------------------|----------------------------------|
| 400       | Eksik/hatali parametre veya X-Install-Seed yok | Header ve body'yi kontrol et |
| 401       | Token gecersiz veya suresi dolmus     | Yeni idToken al, tekrar dene     |
| 402       | Kredi yetersiz                        | Kredi satin al / admin panelden kredi tanimla |
| 403       | Hesap banli veya yetkisiz             | Kullaniciya bilgi ver            |
| 429       | Rate limit asildi                     | Retry-After header'ina bak, bekle|
| 502       | AI provider hatasi                    | Kullaniciya "tekrar dene" goster |
| 503       | Auth servisi yapilandirilmamis        | Backend config kontrol           |

### 9. Rate Limit Header'lari

Her yanit su header'lari icerir:

```
X-RateLimit-Limit: 30         # Penceredeki max istek
X-RateLimit-Remaining: 27     # Kalan istek hakki
Retry-After: 45                # (sadece 429'da) kac saniye beklemeli
```

### 10. Provider Varsayilan Modelleri

| Provider    | Varsayilan Model                         | Kullanim                    |
|-------------|------------------------------------------|-----------------------------|
| fal.ai      | fal-ai/flux/dev/image-to-image           | Gorsel donusturme           |
| replicate   | stability-ai/sdxl                        | Gorsel olusturma            |
| deepseek    | deepseek-chat                            | Multimodal chat             |
| openrouter  | openai/gpt-4o                            | Multimodal chat             |
| gemini      | gemini-1.5-flash                         | Multimodal (vision+text)    |

### 11. Kategori Sistemi

Kategoriler template'leri gruplar. Panelden yonetilir, coklu dil destekler.

Kategori kaydinda `type` alani ile **Fotograf** ve **Video** kategorileri birbirinden ayrilir:

- `type=photo`: Fotograf kategorileri (varsayilan)
- `type=video`: Video kategorileri

Slug benzersizligi `(app_id, type, slug)` composite index ile saglanir.

```
GET /api/v1/categories?app_id=multi-app&lang=en
```

Yanit:
```json
{
  "success": true,
  "data": {
    "categories": [
      { "id": 1, "type": "photo", "slug": "face", "name": "Face", "description": "Face enhancements", "is_active": true, "sort_order": 0 },
      { "id": 2, "type": "video", "slug": "teeth", "name": "Teeth", "description": "Dental corrections", "is_active": true, "sort_order": 1 }
    ]
  }
}
```

> Mobil app tarafinda kategori listesi isterse `type` alanina gore filtreleyebilir (Fotograf/Video sekmeleri gibi).

### 12. Template Sistemi

Backend, her uygulamaya ozel AI sablonlarini panelden yonetmeye olanak tanir:

- **Tekli App** (orn: Dis Teli Temizleme): Tek bir template, sabit prompt
- **Coklu App** (orn: Template Gallery): Birden fazla template, kategori bazli
- **is_featured=true** template'ler slider'da one cikarilabilir

**Reference photos (1 or 2):**

Some templates require **2 reference photos** from the user (e.g. compare/merge style flows). This is controlled by the template field:

- `reference_image_count = 1` (default)
- `reference_image_count = 2`

When `reference_image_count = 2`, the app should collect **two uploads** and call transform with `image_urls`.

**Family photos (Mom, Baby, Dad):**

Some templates require **person photos** (family members) in addition to the main reference photo. This is used for AI models like *flux-pulid* or face-swap style flows where the AI needs separate face references for each family role. The app should request the user to upload separate photos for each checked role and **make them required** before submitting the transform request.

Template fields:

- `require_mom_photo = true/false` — Anne (mom) fotografini zorunlu kilar
- `require_baby_photo = true/false` — Bebek (baby) fotografini zorunlu kilar
- `require_dad_photo = true/false` — Baba (dad) fotografini zorunlu kilar

All three default to `false`. When any is `true`:

1. Mobil app ilgili template icin ayri yukleme alanlari gosterir (ornek: "Mom Photo", "Baby Photo", "Dad Photo")
2. Kullanici tum zorunlu fotograflari yuklemeden transform istegi gonderemez
3. Yuklenen fotograflar transform isteginde `mom_image_url`, `baby_image_url`, `dad_image_url` olarak gonderilir
4. Backend bu URL'leri fal.ai payload'ina dogrudan iletir

Mobil app'te template kart gorunumu (family photo required oldugunda):

```
┌─────────────────────────────────┐
│  ┌─────────┐  ┌─────────┐      │
│  │ BEFORE  │→ │ AFTER   │      │
│  │(resim/  │  │(AI demo │      │
│  │ video)  │  │ sonucu) │      │
│  └─────────┘  └─────────┘      │
│  Template Name          [PRO]  │
│  Action: Image Generation      │
│  👩 Mom Photo   👶 Baby Photo  │
│  👨 Dad Photo   (required)     │
│  1 kredi                       │
└─────────────────────────────────┘
```

Template secildikten sonra yukleme ekrani:

```
┌────────────────────────────────────┐
│  Upload Required Photos           │
│                                    │
│  ┌──────────────────────────┐     │
│  │  [Upload Main Photo]      │     │  ← reference_image (her zaman)
│  └──────────────────────────┘     │
│                                    │
│  ┌──────────────────────────┐     │
│  │  [Upload Mom Photo]  *   │     │  ← require_mom_photo=true ise
│  └──────────────────────────┘     │
│                                    │
│  ┌──────────────────────────┐     │
│  │  [Upload Baby Photo] *   │     │  ← require_baby_photo=true ise
│  └──────────────────────────┘     │
│                                    │
│  ┌──────────────────────────┐     │
│  │  [Upload Dad Photo]  *   │     │  ← require_dad_photo=true ise
│  └──────────────────────────┘     │
│                                    │
│  * = Required                     │
│                                    │
│           [Transform]              │
└────────────────────────────────────┘
```

**Hide from "All" list (for quick-button-only templates):**

Some templates (e.g. "Remove Background") should be accessible via **Quick Buttons** but **not** appear in the main **All** templates feed. This is controlled by:

- `hide_from_all = true`

Behavior:

- `GET /api/v1/templates` (All list) **excludes** `hide_from_all=true` by default
- The template is still returned when filtering by `category_id` or other filters, and can always be opened via Quick Buttons
- If you need to fetch everything for internal tooling: `include_hidden=true`

Example:

```
GET /api/v1/templates?app_id=default&type=photo&include_hidden=true
```

**Aksiyon Tipleri (action_type):**

| Deger               | Aciklama                              | Mobil Davranis                        |
|---------------------|---------------------------------------|---------------------------------------|
| `image_generation`  | AI gorsel olusturma                   | Foto sec → AI donustur → sonuc goster |
| `ai_chat`           | AI sohbet                             | Chat arayuzu ac → metin soru-cevap    |
| `upscale`           | Gorsel buyutme/iyilestirme            | Foto sec → upscale uygula → kaydet    |
| `remove_bg`         | Arka plan kaldirma                    | Foto sec → BG kaldir → seffaf PNG     |
| `photo_restoration` | Eski foto onarma/renklendirme         | Foto sec → restore uygula → kaydet    |

**Before/After Demo Sistemi:**

Her template'in `before_media_url` ve `after_media_url` alanlari vardir:
- **before** = Orijinal gorsel/video (kullaniciya "once" gosterilecek)
- **after** = AI sonuc demosu (kullaniciya "sonra" gosterilecek)
- Her iki alan da `image` veya `video` tipinde olabilir (`before_media_type`, `after_media_type`)

```
Mobil app'te template kart gorunumu:
┌─────────────────────────────────┐
│  ┌─────────┐  ┌─────────┐      │
│  │ BEFORE  │→ │ AFTER   │      │
│  │(resim/  │  │(AI demo │      │
│  │ video)  │  │ sonucu) │      │
│  └─────────┘  └─────────┘      │
│  Template Name          [PRO]  │
│  Action: Image Generation      │
│  1 kredi                       │
└─────────────────────────────────┘
```

### 12.1 Quick Buttons (Slider Alti Kisayollar)

App ana sayfada (Photo/Video sayfalari) slider'in altinda **4 adet buton** gosterebilir. Bu butonlar admin panelden yonetilir.

- **Alanlar**: `title` (EN), `icon_url` (PNG/SVG), `template_id`
- **Filtreleme**: `type=photo|video` ile Photo ve Video sayfalari ayri yonetilir
- **Siralama**: `sort_order` artan

```
GET /api/v1/quick-buttons?app_id=default&type=photo
```

Yanit:
```json
{
  "success": true,
  "data": {
    "buttons": [
      {
        "id": 1,
        "app_id": "default",
        "type": "photo",
        "title": "Remove BG",
        "icon_url": "https://cdn.example.com/icons/remove-bg.svg",
        "template_id": 12,
        "sort_order": 0,
        "is_active": true
      }
    ]
  }
}
```

> Not: Bu buton basliklari global/Ingilizce tutulur (ceviri uygulanmaz).

```
GET /api/v1/templates?app_id=braces-app&lang=tr
GET /api/v1/templates?app_id=multi-app&category_id=2&lang=en
GET /api/v1/templates?app_id=multi-app&featured=true&lang=en
GET /api/v1/templates?action_type=remove_bg&lang=en
```

Yanit:
```json
{
  "success": true,
  "data": {
    "templates": [
      {
        "id": 1,
        "app_id": "braces-app",
        "slug": "remove-braces",
        "name": "Dis Teli Temizle",
        "description": "Fotograftaki dis tellerini dogal sekilde kaldirir",
        "action_type": "image_generation",
        "prompt": "Remove dental braces from teeth, make teeth look natural...",
        "provider": "fal.ai",
        "model": "fal-ai/flux/dev/image-to-image",
        "category_id": 2,
        "before_media_url": "https://cdn.kepa.app/templates/before.jpg",
        "before_media_type": "image",
        "after_media_url": "https://cdn.kepa.app/templates/after.jpg",
        "after_media_type": "image",
        "reference_image_count": 1,
        "require_mom_photo": false,
        "require_baby_photo": false,
        "require_dad_photo": false,
        "credit_cost": 1,
        "is_active": true,
        "is_featured": true,
        "is_premium": false,
        "sort_order": 0
      }
    ]
  }
}
```

> **Not:** `name` ve `description` alanlari `lang` parametresine gore otomatik cevirilmis doner.
> `prompt` CEVIRILMEZ — AI'a her zaman orijinal dilde gonderilir.
> `action_type` mobil app'te hangi UI akilisinin gosterilecegini belirler.

### 12.2 Kullanici Yonetimi ve Kredi Sistemi

Her kullanicinin `credits` (kredi) ve `is_pro` (abonelik) alani vardir:

```
GET /api/v1/me
```

Yanit:
```json
{
  "success": true,
  "data": {
    "uid": "firebase_uid",
    "email": "user@example.com",
    "credits": 5,
    "is_pro": false,
    "usage": { "today_total": 3, "today_success": 3, "all_time": 47 }
  }
}
```

**Mobil app akisi:**
1. Her template'in `credit_cost` alani var (varsayilan: 1)
2. Kullanici transform yapmadan once kredisi kontrol edilir
3. `is_pro` kullanicilar premium template'lere erisebilir
4. Admin panelden istenen kullanicinin kredisi duzenlenebilir

**Admin: kullanici listesi**
```
GET /api/admin/users?page=1&limit=50&search=email@
```
```json
{
  "data": {
    "users": [
      { "id": 1, "email": "u@e.com", "credits": 5, "is_pro": true, "total_usage": 47 }
    ]
  }
}
```

**Admin: kredi/pro guncelle**
```
PUT /api/admin/users/1
{ "credits": 100, "is_pro": true }
```

### 13. Slider / Banner Sistemi

App ana ekranindaki slider. One cikan template'ler, ozel gun cerceceleri ve promosyonlar icin:

- `template_id` ile bir template'e baglanabilir (dokunulunca o template acilir)
- `frame_url` ile seffaf PNG cerceve overlay eklenir (ozel gunler icin)
- `starts_at` / `ends_at` ile tarih araliginda otomatik gosterilir
- Tarih disinda olan slider item'lar otomatik filtrelenir

```
GET /api/v1/slider?app_id=braces-app&lang=en
```

Yanit:
```json
{
  "success": true,
  "data": {
    "slider": [
      {
        "id": 1,
        "app_id": "braces-app",
        "template_id": 1,
        "title": "Try Braces Removal!",
        "description": "See your smile without braces",
        "image_url": "https://cdn.kepa.app/slider/hero.jpg",
        "frame_url": null,
        "deep_link": "template://remove-braces",
        "sort_order": 0,
        "is_active": true,
        "starts_at": null,
        "ends_at": null
      },
      {
        "id": 2,
        "title": "New Year Special!",
        "image_url": "https://cdn.kepa.app/slider/newyear-bg.jpg",
        "frame_url": "https://cdn.kepa.app/slider/newyear-frame.png",
        "starts_at": "2026-12-25T00:00:00Z",
        "ends_at": "2027-01-05T23:59:59Z"
      }
    ]
  }
}
```

**Mobil app'te frame kullanimi:**
```
Slider gorunumu:
┌──────────────────────────┐
│  [image_url arka plan]   │
│  ┌──────────────────┐    │
│  │ [frame_url PNG    │    │  ← seffaf PNG overlay (ozel gun cercevesi)
│  │  uzerine bindirir]│    │
│  └──────────────────┘    │
│  Title                   │
│  Description             │
└──────────────────────────┘
```

### 14. Onboarding Medya

Uygulama acilisindaki onboarding ekranlari icin video/gorsel yonetimi:

```
GET /api/v1/onboarding?app_id=braces-app&lang=tr
```

Yanit:
```json
{
  "success": true,
  "data": {
    "onboarding": [
      {
        "id": 1,
        "app_id": "braces-app",
        "type": "video",
        "title": "Hos Geldin!",
        "description": "AI ile dis tellerini temizle",
        "media_url": "https://cdn.kepa.app/onboarding/welcome.mp4",
        "thumbnail_url": "https://cdn.kepa.app/onboarding/thumb1.jpg",
        "sort_order": 0
      }
    ]
  }
}
```

### 14.1 Onboarding Kullanici Yorumlari

Onboarding akisinda bir sayfa App Store tarzi kullanici yorumlari gosterir.
Admin panelden manuel eklenir: kullanici adi, profil fotosu, yorum metni, puan (1-5 yildiz).

```
GET /api/v1/reviews?lang=tr
```

Yanit:
```json
{
  "success": true,
  "data": {
    "reviews": [
      {
        "id": 1,
        "nickname": "Sarah M.",
        "photo_url": "https://cdn.kepa.app/reviews/sarah.jpg",
        "review": "Bu uygulama harika! Dis tellerimi kaldirdigini gormek muhteemdi.",
        "rating": 5,
        "sort_order": 0,
        "is_active": true
      }
    ]
  }
}
```

**Mobil app'te onboarding yorum sayfasi:**
```
Onboarding Review Page:
┌──────────────────────────────────────┐
│  "Kullanicilarimiz ne diyor?"        │
│                                      │
│  ┌──────────────────────────────┐    │
│  │ (o) Sarah M.  ★★★★★         │    │
│  │ "Bu uygulama harika! Dis    │    │
│  │  tellerimi kaldirdigini..." │    │
│  └──────────────────────────────┘    │
│                                      │
│  ┌──────────────────────────────┐    │
│  │ (o) Alex K.   ★★★★★         │    │
│  │ "Sonuclar cok gercekci.     │    │
│  │  Herkese tavsiye ederim!"   │    │
│  └──────────────────────────────┘    │
│                                      │
│           [Devam Et]                 │
└──────────────────────────────────────┘
```

> `review` alani `lang` parametresine gore cevirilmis doner.
> `nickname` ve `photo_url` cevirilmez.
> Admin panelde "Cevir" butonuyla yorum metni tum dillere cevirilir.

### 15. Coklu Dil (i18n) Sistemi

Tum icerikler (template, kategori, slider, onboarding, review) admin panelden tek tusla 16 dile cevirilir.
Ceviri DeepSeek AI ile yapilir, veritabaninda saklanir.

**Desteklenen diller:**

| Kod | Dil        | Kod | Dil        |
|-----|------------|-----|------------|
| en  | English    | ja  | Japanese   |
| tr  | Turkce     | ko  | Korean     |
| de  | Deutsch    | zh  | Chinese    |
| fr  | Francais   | ar  | Arabic     |
| es  | Espanol    | ru  | Russian    |
| pt  | Portugues  | hi  | Hindi      |
| it  | Italiano   | nl  | Nederlands |
| sv  | Svenska    | pl  | Polski     |

**Mobil app kullanimi:**

```
GET /api/v1/templates?app_id=xx&lang=de   → name/description Almanca doner
GET /api/v1/categories?app_id=xx&lang=fr  → name/description Fransizca doner
GET /api/v1/slider?app_id=xx&lang=ja      → title/description Japonca doner
GET /api/v1/onboarding?app_id=xx&lang=ko  → title/description Korece doner
GET /api/v1/reviews?lang=de              → review metni Almanca doner
GET /api/v1/languages                     → desteklenen dil listesi
```

> `lang` parametresi gonderilmezse orijinal (panelde girilen) deger doner.
> `prompt` alani HICBIR ZAMAN cevirilmez — AI'a orijinal gonder.
> Ceviriler paneldeki Cevir butonuyla yapilir ve veritabaninda saklanir.

**Desteklenen dilleri al:**
```
GET /api/v1/languages
```
```json
{
  "success": true,
  "data": {
    "languages": [
      { "code": "en", "name": "English" },
      { "code": "tr", "name": "Turkce" },
      { "code": "de", "name": "Deutsch" }
    ]
  }
}
```

### 16. Tipik App Akisi

```
1. Uygulama acilir
2. GET /api/v1/onboarding?app_id=xx&lang=DEVICE_LANG  → onboarding gosterilir
2b. GET /api/v1/reviews?lang=DEVICE_LANG               → yorum sayfasi gosterilir
3. Firebase Auth ile giris                              → idToken alinir
4. GET /api/v1/me                                       → profil + credits + is_pro
5. GET /api/v1/categories?app_id=xx&lang=DEVICE_LANG   → kategoriler listelenir
6. GET /api/v1/templates?app_id=xx&lang=DEVICE_LANG    → template'ler listelenir
7. GET /api/v1/slider?app_id=xx&lang=DEVICE_LANG       → slider/banner gosterilir
7b. GET /api/v1/quick-buttons?app_id=xx&type=photo      → slider alti butonlar
8. Kullanici template secer (before/after demosunu gorur)
8b. Aspect ratio secimi:
    - template.supported_aspect_ratios virgul ile split edilir → butonlar gosterilir
    - template.aspect_ratio varsayilan secili olarak gelir
    - Kullanici isterse farkli bir oran secer
    - Secilen oran transform isteginde params.aspect_ratio olarak gonderilir
8c. Family photo gereksinimleri kontrolu:
    - template.require_mom_photo, require_baby_photo, require_dad_photo alanalri kontrol edilir
    - Herhangi biri true ise, app ilgili rol icin ayri yukleme alani gosterir (Mom Photo, Baby Photo, Dad Photo)
    - Tum zorunlu fotograflar yuklenmeden transform butonu aktif degildir
    - Yuklenen fotograflar once POST /api/v1/upload ile R2'ye yuklenir, ardindan transform isteginde mom_image_url, baby_image_url, dad_image_url olarak gonderilir
9. action_type'a gore UI akisi belirlenir:

   image_generation:  Foto sec → ratio sec → upload → transform → sonuc
   ai_chat:           Chat UI ac → mesaj gonder → cevap al
   upscale:           Foto sec → upload → transform (upscale model) → sonuc
   remove_bg:         Foto sec → upload → transform (bg remove) → seffaf PNG
   photo_restoration: Foto sec → upload → transform (restore model) → sonuc

10. Kredi kontrolu: credits >= template.credit_cost mi?
    - Evet: devam et
    - Hayir: "Yetersiz kredi" goster, satin alma yonlendir
11. POST /api/v1/upload                                → foto R2'ye yuklenir
12. POST /api/v1/transform                             → template.prompt kullanilir
13. Sonuc gosterilir (kalici CDN URL)
14. GET /api/v1/history                                → gecmis gosterilir
```

> **DEVICE_LANG:** iOS'ta `Locale.current.language.languageCode?.identifier`,
> Android'de `Locale.getDefault().language` degerini kullan.

### 16.1 Action Type'a Gore Mobil App Buton Yapisi

```
Ana Ekran (Template Gallery):
┌─────────────────────────────────────────┐
│ [Slider / Banner]                       │
├─────────────────────────────────────────┤
│ Quick Buttons: [RemBG] [Upscale] [...]  │
├─────────────────────────────────────────┤
│ Kategoriler:  [Tumu] [Yuz] [Dis] [BG]  │
├─────────────────────────────────────────┤
│ ┌───────┐ ┌───────┐ ┌───────┐          │
│ │Before │ │Before │ │Before │          │
│ │→After │ │→After │ │→After │          │
│ │ImgGen │ │Upscale│ │RemBG  │          │
│ └───────┘ └───────┘ └───────┘          │
└─────────────────────────────────────────┘
```

Bu butonlara tiklandiginda:
```swift
// iOS ornegi
switch template.actionType {
case "image_generation":
    navigateToTransformScreen(template)
case "ai_chat":
    navigateToChatScreen(template)
case "upscale":
    navigateToUpscaleScreen(template)
case "remove_bg":
    navigateToRemoveBgScreen(template)
case "photo_restoration":
    navigateToRestoreScreen(template)
}
```

### 16.2 Aspect Ratio (En-Boy Orani) Sistemi

Her template icin bir varsayilan ve desteklenen aspect ratio listesi bulunur:

| Alan | Tip | Aciklama |
|------|-----|----------|
| `aspect_ratio` | string | Varsayilan oran (ornek: `"1:1"`) |
| `supported_aspect_ratios` | string | Virgul ile ayrilmis liste (ornek: `"1:1,4:5,9:16,16:9,3:4,4:3"`) |

**Desteklenen oranlar:**
- `1:1` — Kare (Instagram post)
- `4:5` — Dikey (Instagram portrait)
- `9:16` — Tam dikey (Story/Reels/TikTok)
- `16:9` — Tam yatay (YouTube thumbnail)
- `3:4` — Klasik dikey (Portre foto)
- `4:3` — Klasik yatay (Landscape foto)

**iOS Akisi:**
```
1. Template secilir
2. template.supported_aspect_ratios split edilir → ratio butonlari gosterilir
3. template.aspect_ratio varsayilan secili olarak isaretlenir
4. Kullanici isterse farkli ratio secer
5. Transform isteginde params.aspect_ratio olarak gonderilir
```

**Ornek API response (GET /api/v1/templates):**
```json
{
  "id": 1,
  "name": "Face Enhance",
  "aspect_ratio": "1:1",
  "supported_aspect_ratios": "1:1,4:5,9:16",
  "action_type": "image_generation",
  ...
}
```

**Ornek transform istegi (POST /api/v1/transform):**
```json
{
  "provider": "fal.ai",
  "image_url": "https://cdn.example.com/uploads/photo.jpg",
  "prompt": "Enhance face details...",
  "params": {
    "aspect_ratio": "9:16"
  }
}
```

### 16.3 Desteklenen Modeller ve Konfigurasyonu

Backend, fal.ai uzerinden farkli model tiplerine otomatik adapte olur. Admin panelde template olustururken
`provider` = `fal.ai` ve `model` alanina asagidaki model path'lerinden biri yazilir.

#### Foto: Flux PuLID (Kisi bazli gorsel uretim)

| Alan | Deger |
|------|-------|
| Provider | `fal.ai` |
| Model | `fal-ai/flux-pulid` |
| image_url alanı | Backend otomatik olarak `reference_image_url`'e cevirir |
| aspect_ratio | Backend otomatik olarak `image_size` enum'a cevirir |

**image_size mapping (otomatik):**
| Bizim Ratio | PuLID image_size |
|-------------|------------------|
| `1:1` | `square_hd` |
| `4:5` | `portrait_4_3` |
| `9:16` | `portrait_16_9` |
| `16:9` | `landscape_16_9` |
| `3:4` | `portrait_4_3` |
| `4:3` | `landscape_4_3` |

**Ornek template konfigurasyonu (Admin):**
```json
{
  "name": "Face Transform",
  "provider": "fal.ai",
  "model": "fal-ai/flux-pulid",
  "action_type": "image_generation",
  "prompt": "a professional photo of the person, high quality...",
  "aspect_ratio": "1:1",
  "supported_aspect_ratios": "1:1,4:5,9:16,16:9,3:4,4:3",
  "params": ""
}
```

> **Not:** `params` bos birakilabilir — her model kendi varsayilan degerleriyle calisir.
> Sadece varsayilanlari degistirmek istersen JSON yazarsin, ornek: `{"num_inference_steps": 30}`

**PuLID desteklenen params (tumu opsiyonel):**
- `num_inference_steps` (int, default: 20)
- `guidance_scale` (float, default: 4)
- `negative_prompt` (string)
- `id_weight` (float, default: 1)
- `true_cfg` (float, default: 1)
- `seed` (int)
- `max_sequence_length` (128 | 256 | 512)

#### Video: Kling 1.5 Pro (Gorselten videoya)

| Alan | Deger |
|------|-------|
| Provider | `fal.ai` |
| Model | `fal-ai/kling-video/v1.5/pro/image-to-video` |
| aspect_ratio | Direkt gecerli (`16:9`, `9:16`, `1:1`) |
| Output | `video.url` — Backend otomatik olarak cikartir |

**aspect_ratio mapping (otomatik):**
| Bizim Ratio | Kling Ratio |
|-------------|-------------|
| `1:1` | `1:1` |
| `9:16`, `4:5`, `3:4` | `9:16` |
| `16:9`, `4:3` | `16:9` |

> Kling sadece 3 oran destekler (1:1, 9:16, 16:9). Diger oranlar en yakin desteklenen orana yuvarlanir.

**Ornek template konfigurasyonu (Admin):**
```json
{
  "name": "Photo to Video",
  "provider": "fal.ai",
  "model": "fal-ai/kling-video/v1.5/pro/image-to-video",
  "action_type": "image_generation",
  "prompt": "Cinematic motion, camera slowly zooms in...",
  "aspect_ratio": "16:9",
  "supported_aspect_ratios": "1:1,9:16,16:9",
  "params": ""
}
```

> **Not:** `params` bos birakilabilir — Kling varsayilan 5sn, 16:9 ile calisir.
> Duration'i 10sn yapmak istersen: `{"duration": "10"}`

**Kling desteklenen params (tumu opsiyonel):**
- `duration` (string: "5" veya "10", default: "5")
- `aspect_ratio` (otomatik cevirilir, manuel de gonderilebilir)
- `negative_prompt` (string, default: "blur, distort, and low quality")
- `cfg_scale` (float, default: 0.5)
- `tail_image_url` (string, video sonu icin gorsel)

> **Timeout:** Video modelleri uzun surer. Backend timeout'u 300 saniyeye ayarlanmistir.
> iOS tarafinda da timeout'u buna uygun (en az 5 dakika) tutun.

#### Model Ekleme Rehberi

Yeni bir fal.ai modeli eklemek icin:
1. Admin panelde template olustur, `model` alanina fal.ai model path'ini yaz
2. `params` JSON'a modele ozel parametreleri ekle
3. Backend otomatik olarak:
   - `aspect_ratio` → model formatina cevirir (image_size veya aspect_ratio)
   - `image_url` → model'in bekledigini alana map'ler (reference_image_url vs.)
   - Sonucu hem `images[].url` hem `video.url` formatinda cikarir

### 17. iOS (Swift) Ornek Kullanim

```swift
struct TransformRequest: Codable {
    let provider: String
    let model: String?
    let imageUrl: String
    let imageUrls: [String]?
    let momImageUrl: String?
    let babyImageUrl: String?
    let dadImageUrl: String?
    let prompt: String
    let params: [String: AnyCodable]?

    enum CodingKeys: String, CodingKey {
        case provider, model, prompt, params
        case imageUrl = "image_url"
        case imageUrls = "image_urls"
        case momImageUrl = "mom_image_url"
        case babyImageUrl = "baby_image_url"
        case dadImageUrl = "dad_image_url"
    }
}

struct APIResponse<T: Codable>: Codable {
    let success: Bool
    let data: T?
    let error: String?
}

struct TransformResult: Codable {
    let resultUrl: String
    let provider: String
    let model: String

    enum CodingKeys: String, CodingKey {
        case resultUrl = "result_url"
        case provider, model
    }
}

// Kullanim
func transform(imageUrl: String, prompt: String) async throws -> TransformResult {
    let token = try await Auth.auth().currentUser?.getIDToken()
    let installSeed = InstallSeedStore.shared.getOrCreate()

    var request = URLRequest(url: URL(string: "\(baseURL)/api/v1/transform")!)
    request.httpMethod = "POST"
    request.setValue("Bearer \(token ?? "")", forHTTPHeaderField: "Authorization")
    request.setValue(installSeed, forHTTPHeaderField: "X-Install-Seed")
    request.setValue("application/json", forHTTPHeaderField: "Content-Type")

    let body = TransformRequest(
        provider: "fal.ai",
        model: nil,
        imageUrl: imageUrl,
        imageUrls: nil,
        prompt: prompt,
        params: nil
    )
    request.httpBody = try JSONEncoder().encode(body)

    let (data, _) = try await URLSession.shared.data(for: request)
    let response = try JSONDecoder().decode(APIResponse<TransformResult>.self, from: data)

    guard response.success, let result = response.data else {
        throw APIError.serverError(response.error ?? "Unknown error")
    }
    return result
}
```

If a template requires 2 photos:

```swift
let body = TransformRequest(
    provider: template.provider,
    model: template.model,
    imageUrl: imageUrls[0],          // keep for backward compatibility
    imageUrls: imageUrls,            // ["url1", "url2"]
    prompt: template.prompt,
    params: nil
)
```

Aspect ratio secimi ile kullanim:

```swift
// Template'den desteklenen oranlari al
let ratios = template.supportedAspectRatios
    .components(separatedBy: ",")     // ["1:1", "4:5", "9:16", ...]

// Varsayilan oran
let defaultRatio = template.aspectRatio  // "1:1"

// Kullanici bir oran sectikten sonra transform istegi
let body = TransformRequest(
    provider: template.provider,
    model: template.model,
    imageUrl: imageUrl,
    imageUrls: nil,
    prompt: template.prompt,
    params: ["aspect_ratio": selectedRatio]  // kullanicinin sectigi oran
)
```

Family photo (anne/bebek/baba fotograflari) ile kullanim:

```swift
// Template'in family photo gereksinimlerini kontrol et
let needsMom = template.requireMomPhoto   // Bool
let needsBaby = template.requireBabyPhoto  // Bool
let needsDad = template.requireDadPhoto    // Bool

// UI'da her require_*_photo = true icin ayri yukleme alani goster
// Tum zorunlu fotograflar yuklendikten sonra transform istegini gonder

let body = TransformRequest(
    provider: template.provider,
    model: template.model,
    imageUrl: mainPhotoUrl,        // ana gorsel (her zaman gerekli)
    imageUrls: nil,
    momImageUrl: needsMom ? momPhotoUrl : nil,    // anne fotografi
    babyImageUrl: needsBaby ? babyPhotoUrl : nil,  // bebek fotografi
    dadImageUrl: needsDad ? dadPhotoUrl : nil,      // baba fotografi
    prompt: template.prompt,
    params: ["aspect_ratio": selectedRatio]
)
```

Video template ile kullanim (Kling):

```swift
// Video template icin timeout'u artir
var request = URLRequest(url: URL(string: "\(baseURL)/api/v1/transform")!)
request.timeoutInterval = 300 // 5 dakika — video modelleri uzun surer

let body = TransformRequest(
    provider: template.provider,
    model: template.model,                  // "fal-ai/kling-video/v1.5/pro/image-to-video"
    imageUrl: imageUrl,
    imageUrls: nil,
    prompt: template.prompt,
    params: [
        "aspect_ratio": selectedRatio,      // "16:9", "9:16", "1:1"
        "duration": "5"                     // "5" veya "10" saniye
    ]
)
// result_url .mp4 video URL'i doner (R2 CDN'de kalici)
```

### 18. Android (Kotlin) Ornek Kullanim

```kotlin
// Retrofit Interface
interface AIBridgeApi {
    @POST("api/v1/transform")
    suspend fun transform(
        @Header("Authorization") token: String,
        @Header("X-Install-Seed") installSeed: String,
        @Body request: TransformRequest
    ): ApiResponse<TransformResult>
}

// Data Classes
data class TransformRequest(
    val provider: String,
    val model: String? = null,
    @SerializedName("image_url") val imageUrl: String,
    @SerializedName("image_urls") val imageUrls: List<String>? = null,
    @SerializedName("mom_image_url") val momImageUrl: String? = null,
    @SerializedName("baby_image_url") val babyImageUrl: String? = null,
    @SerializedName("dad_image_url") val dadImageUrl: String? = null,
    val prompt: String,
    val params: Map<String, Any>? = null
)

data class TransformResult(
    @SerializedName("result_url") val resultUrl: String,
    val provider: String,
    val model: String
)

data class ApiResponse<T>(
    val success: Boolean,
    val data: T?,
    val error: String?
)

// Repository
class TransformRepository(private val api: AIBridgeApi) {
    suspend fun transform(imageUrl: String, prompt: String): Result<TransformResult> {
        val token = Firebase.auth.currentUser?.getIdToken(false)?.await()?.token
            ?: return Result.failure(Exception("Not authenticated"))
        val installSeed = installSeedStore.getOrCreate()

        val response = api.transform(
            token = "Bearer $token",
            installSeed = installSeed,
            request = TransformRequest(
                provider = "fal.ai",
                imageUrl = imageUrl,
                prompt = prompt
            )
        )

        return if (response.success && response.data != null) {
            Result.success(response.data)
        } else {
            Result.failure(Exception(response.error ?: "Unknown error"))
        }
    }
}
```

---

## Push Notifications (FCM)

Backend, iOS ve Android kullanicilarina manuel bildirim gondermek icin **Firebase Cloud Messaging (FCM)** kullanir. Ayri bir servis gerekmez — panelden yapilandirilan ayni Firebase service account ile calisir. Amac: kullaniciyi ara ara uygulamaya geri cekmek (yeni template, ozel gun kampanyasi, hatirlatma vb.) icin admin panelden **elle bildirim yazip gondermek**.

### Akis

```
1. iOS app Firebase SDK'dan FCM token al (APNs uzerinden)
2. App, idToken + X-Install-Seed ile POST /api/v1/device-token cagrir  →  token backend'e kaydedilir
3. Admin panelden "Push Notifications" sekmesinden bildirim yazar
4. Backend tum eslesen token'lara FCM multicast gonderir
5. Gecersiz (unregistered/invalid) token'lar veritabanindan otomatik silinir
```

### 1. iOS Entegrasyonu

**Xcode:**
- `Signing & Capabilities` → **Push Notifications** capability ekle
- `Signing & Capabilities` → **Background Modes** → **Remote notifications** ac
- Apple Developer Portal'da APNs Key (`.p8`) olustur → Firebase Console → Cloud Messaging → APNs key olarak yukle

**Zorunlu uyumluluk checklist'i (iOS):**
1. `installSeed` degerini **Keychain**'de kalici tut (reinstall sonrasi ayni deger korunmali).
2. Tum `/api/v1/*` isteklerinde `X-Install-Seed` header'ini gonder.
3. Ozellikle auth sonrasi ilk `GET /api/v1/me`, `POST /api/v1/transform`, `POST /api/v1/upload`, `POST /api/v1/device-token` cagrilarinda header zorunlu olsun.

**InstallSeedStore (Swift, ornek):**

```swift
import Foundation
import Security

final class InstallSeedStore {
    static let shared = InstallSeedStore()
    private let service = "com.luris.install-seed"
    private let account = "stable-seed"

    func getOrCreate() -> String {
        if let existing = read() { return existing }
        let seed = UUID().uuidString + "-" + UUID().uuidString
        save(seed)
        return seed
    }

    private func read() -> String? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne
        ]
        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)
        guard status == errSecSuccess,
              let data = item as? Data,
              let value = String(data: data, encoding: .utf8) else { return nil }
        return value
    }

    private func save(_ value: String) {
        let data = Data(value.utf8)
        let addQuery: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecValueData as String: data
        ]
        let status = SecItemAdd(addQuery as CFDictionary, nil)
        if status == errSecDuplicateItem {
            let updateQuery: [String: Any] = [
                kSecClass as String: kSecClassGenericPassword,
                kSecAttrService as String: service,
                kSecAttrAccount as String: account
            ]
            let attrsToUpdate: [String: Any] = [kSecValueData as String: data]
            SecItemUpdate(updateQuery as CFDictionary, attrsToUpdate as CFDictionary)
        }
    }
}
```

**AppDelegate / SwiftUI:**

```swift
import Firebase
import FirebaseMessaging
import UserNotifications

@main
struct LurisApp: App {
    init() {
        FirebaseApp.configure()
    }
    // ...
}

final class AppDelegate: NSObject, UIApplicationDelegate, UNUserNotificationCenterDelegate, MessagingDelegate {
    func application(_ application: UIApplication, didFinishLaunchingWithOptions opts: [UIApplication.LaunchOptionsKey : Any]? = nil) -> Bool {
        UNUserNotificationCenter.current().delegate = self
        Messaging.messaging().delegate = self

        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .badge, .sound]) { _, _ in }
        DispatchQueue.main.async { application.registerForRemoteNotifications() }
        return true
    }

    func application(_ application: UIApplication, didRegisterForRemoteNotificationsWithDeviceToken t: Data) {
        Messaging.messaging().apnsToken = t
    }

    // FCM token teslim edildi
    func messaging(_ messaging: Messaging, didReceiveRegistrationToken fcmToken: String?) {
        guard let token = fcmToken else { return }
        Task { await registerDeviceToken(token) }
    }
}

func registerDeviceToken(_ token: String) async {
    guard let idToken = try? await Auth.auth().currentUser?.getIDToken() else { return }
    let installSeed = InstallSeedStore.shared.getOrCreate()
    var req = URLRequest(url: URL(string: "\(baseURL)/api/v1/device-token")!)
    req.httpMethod = "POST"
    req.setValue("Bearer \(idToken)", forHTTPHeaderField: "Authorization")
    req.setValue(installSeed, forHTTPHeaderField: "X-Install-Seed")
    req.setValue("application/json", forHTTPHeaderField: "Content-Type")
    let body = [
        "token": token,
        "platform": "ios",
        "app_id": "default",
        "locale": Locale.current.language.languageCode?.identifier ?? "en"
    ]
    req.httpBody = try? JSONSerialization.data(withJSONObject: body)
    _ = try? await URLSession.shared.data(for: req)
}
```

> **Logout'ta** aynı endpoint'e `DELETE` at (token'i body'de gonder). Boylece cikis yapan kullaniciya bildirim gitmez.

### 2. Token Kayit Endpoint'i

```
POST /api/v1/device-token
Authorization: Bearer <firebase_id_token>
X-Install-Seed: <stable_install_seed>
Content-Type: application/json

{
  "token":    "APA91b...fcm_token_str",
  "platform": "ios",            // ios | android
  "app_id":   "default",        // opsiyonel
  "locale":   "en"              // opsiyonel
}
```

Yanit:
```json
{
  "success": true,
  "data": { "id": 42, "registered": true }
}
```

### 3. Token Silme (Logout)

```
DELETE /api/v1/device-token
Authorization: Bearer <firebase_id_token>
X-Install-Seed: <stable_install_seed>
Content-Type: application/json

{ "token": "APA91b...fcm_token_str" }
```

### 4. Admin Panel — Manuel Bildirim Gonder

Panelde **Push Notifications** sekmesi:

- **Title** (max 80 kar)
- **Body** (max 200 kar)
- **Platform**: `ios`, `android` veya `all`
- **App ID** (opsiyonel, cok-app modunda filtreleme)
- **Deep link** (opsiyonel, `data.deep_link` olarak gonderilir)
- **Target UIDs** (opsiyonel, sadece belirli kullanicilara)

Admin API:

```
POST /api/admin/notifications/send
Authorization: Bearer <admin_id_token>
Content-Type: application/json

{
  "title":       "Seni ozledik! 👋",
  "body":        "Yeni template'leri kesfetmek icin geri don.",
  "platform":    "ios",                 // ios | android | all
  "app_id":      "default",             // opsiyonel
  "deep_link":   "luris://home",        // opsiyonel (app'te tiklanmaya yonlendirme)
  "target_uids": [],                    // opsiyonel, bosken hepsine gider
  "data":        { "campaign": "winback" }
}
```

Yanit:
```json
{
  "success": true,
  "data": {
    "sent":           142,
    "failed":         3,
    "total_targets":  145,
    "pruned_invalid": 3
  }
}
```

> **Gecersiz token'lar** (`Unregistered`, `InvalidArgument`) FCM tarafindan reddedildiginde otomatik silinir; bir dahaki gondermede listeye dahil edilmezler.

### 5. Deep Link Yonlendirmesi (iOS)

```swift
func userNotificationCenter(_ center: UNUserNotificationCenter,
                            didReceive response: UNNotificationResponse,
                            withCompletionHandler completionHandler: @escaping () -> Void) {
    let data = response.notification.request.content.userInfo
    if let link = data["deep_link"] as? String, let url = URL(string: link) {
        // kendi router'ina yonlendir
        AppRouter.shared.handle(url)
    }
    completionHandler()
}
```

### 6. Gereksinimler

- `FIREBASE_CONFIG_PATH` (service account JSON) **zorunlu** — FCM bu kimlikle calisir
- Firebase Console → Cloud Messaging → **APNs Authentication Key** yuklenmis olmali (iOS icin)
- Panelde "Push Notifications" sekmesinde **FCM status** satiri `Ready` gorunmuyorsa service account dosyasi eksiktir

---

## Ortam Degiskenleri

| Degisken               | Zorunlu    | Varsayilan                      | Aciklama                              |
|------------------------|------------|---------------------------------|---------------------------------------|
| `PORT`                 | Hayir      | 3000                            | Sunucu portu                          |
| `ADMIN_EMAIL`          | Evet       | -                               | Admin panel erisim emaili             |
| `FIREBASE_CONFIG_PATH` | Opsiyonel  | ./firebase-service-account.json | Firebase SA JSON (mobil API icin)     |
| `FIREBASE_WEB_API_KEY` | Evet       | -                               | Firebase web API key (panel icin)     |
| `FIREBASE_AUTH_DOMAIN` | Evet       | -                               | Firebase auth domain                  |
| `FIREBASE_PROJECT_ID`  | Evet       | -                               | Firebase project ID                   |
| `FIREBASE_APP_ID`      | Evet       | -                               | Firebase app ID                       |
| `FAL_AI_KEY`           | Hayir      | -                               | fal.ai API key                        |
| `REPLICATE_KEY`        | Hayir      | -                               | Replicate API key                     |
| `DEEPSEEK_KEY`         | Hayir      | -                               | DeepSeek API key (AI + ceviri)       |
| `OPENROUTER_KEY`       | Hayir      | -                               | OpenRouter API key                    |
| `GEMINI_KEY`           | Hayir      | -                               | Google Gemini API key                 |
| `REVENUECAT_API_KEY`   | Hayir      | -                               | RevenueCat API key                    |
| `REVENUECAT_PROJECT_ID`| Hayir      | -                               | RevenueCat project ID                 |
| `S3_ENDPOINT`          | Hayir      | -                               | R2/S3 endpoint URL                    |
| `S3_ACCESS_KEY_ID`     | Hayir      | -                               | R2/S3 access key                      |
| `S3_SECRET_ACCESS_KEY` | Hayir      | -                               | R2/S3 secret key                      |
| `S3_REGION`            | Hayir      | auto                            | R2/S3 region                          |
| `S3_BUCKET_NAME`       | Hayir      | -                               | R2/S3 bucket adi                      |
| `S3_PUBLIC_URL`        | Hayir      | -                               | CDN public URL (orn: https://cdn.app) |
| `RATE_LIMIT_MAX`       | Hayir      | 30                              | Pencere basina max istek              |
| `RATE_LIMIT_WINDOW`    | Hayir      | 60                              | Rate limit penceresi (saniye)         |
| `INITIAL_CREDITS`      | Hayir      | 2                               | Yeni install seed icin tek seferlik acilis kredisi |
| `CORS_ALLOW_ORIGINS`   | Hayir      | *                               | Izin verilen origin'ler (virgul ile)  |

### Zorunluluk Notlari

- **Panel icin minimum:** `ADMIN_EMAIL`, `FIREBASE_WEB_API_KEY`, `FIREBASE_AUTH_DOMAIN`, `FIREBASE_PROJECT_ID`, `FIREBASE_APP_ID`
- **Mobil API icin:** Yukaridakilere ek olarak `FIREBASE_CONFIG_PATH`, en az 1 AI provider key ve her istekte `X-Install-Seed` header'i
- **AI provider key'leri:** Placeholder degerler (SONRA, your_key, vb.) otomatik filtrelenir
- **R2/S3 storage:** Tum 6 S3 degiskeni tanimlanirsa aktif olur. Tanimlanmazsa upload lokal, result URL'leri provider'dan gelir (gecici)
- **CORS:** Production'da `CORS_ALLOW_ORIGINS` ile spesifik origin'lere kisitlanmasi onerilir

---

## Guvenlik

### Mimari Guvenlik

- **API key izolasyonu:** Tum AI provider key'leri sunucu tarafinda `.env` ile yonetilir, istemciye hicbir zaman iletilmez
- **Firebase Auth:** Mobil API'da Firebase Admin SDK ile tam token dogrulama (imza, issuer, audience, expiry). Admin panelde Google x509 sertifikalari ile lightweight JWT dogrulama
- **Install-seed korumasi:** Yeni kullanici olusumu icin `X-Install-Seed` zorunludur. Baslangic kredisi ayni seed icin sadece bir kez verilir (`install_credit_claims`)
- **Admin erisim:** `ADMIN_EMAIL` ile kisitlanir, `AdminOnly` middleware ile cift kontrol
- **Ban sistemi:** Hem mobil API hem admin panel auth middleware'inde `isUserBanned` kontrolu aktif
- **Rate limiting:** UID/IP bazli pencere sistemi, abuse ve maliyet kontrolu saglar
- **Sunucu tarafi kredi dusumu:** `/api/v1/transform` basarili islemde kredi tuketir, kredi yetersizse `402` doner
- **Placeholder key reddi:** `SONRA`, `your_key` gibi degerler otomatik reddedilir

### Hardening Onlemleri

- **Hata mesaji gizleme:** 5xx hatalarinda istemciye genel mesaj doner, detay sadece sunucu loglarinda saklanir. Provider hatalari `err.Error()` ile disariya sizdirilmaz
- **ServerHeader gizleme:** Response header'larinda sunucu bilgisi ifsa edilmez (`ServerHeader: ""`)
- **SSRF / DNS Rebinding koruması:** `UploadFromURL` custom `http.Transport` ile DNS cozumlemesi sonrasi resolved IP'yi kontrol eder. Private IP araliklari (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 127.0.0.0/8, ::1, fc00::/7) ve localhost engellenir
- **SQL injection koruması:** Tum GORM `.Where()` cagrilari parameterized (`?` placeholder). String concatenation ile SQL olusturulmaz
- **Security header'lari:** `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `X-XSS-Protection`, `Referrer-Policy`, `Permissions-Policy`
- **CORS konfigurasyonu:** `CORS_ALLOW_ORIGINS` env degiskeni ile kisitlanabilir
- **SRI (Subresource Integrity):** Admin panel CDN script'leri (Firebase, Alpine.js, Chart.js) pinlenmiş versiyonlarla ve `integrity` hash'leri ile yuklenir
- **XSS koruması:** Alpine.js `x-text` kullanilir (`x-html` sadece statik SVG icon'larda)
- **Upload guvenli:** Dosya tipi kontrolu, boyut limiti, R2'ye yukleme

---

## Deploy (Coolify)

1. GitHub repo'yu Coolify'a bagla
2. Build Pack: **Dockerfile** sec
3. Port: **3000**
4. Environment Variables ekle (yukaridaki tablo)
5. Volume Mount: `bubsiebackend-data` -> `/app/data` (SQLite kaliciligi)
6. Firebase SA gerekiyorsa: Storage mount ile `/app/firebase-service-account.json`
7. Auto-deploy: main branch'e push = otomatik deploy

---

## DB Semalari

### users
| Kolon        | Tip       | Aciklama                         |
|--------------|-----------|----------------------------------|
| id           | uint (PK) | Auto increment                   |
| firebase_uid | string    | Unique index                     |
| email        | string    | Index                            |
| name         | string    |                                  |
| photo_url    | string    |                                  |
| credits      | int       | Kullanici kredisi (ilk install seed claim'inde `INITIAL_CREDITS`) |
| is_pro       | bool      | Pro abonelik durumu              |
| is_banned    | bool      | Engelli kullanici (Index)        |
| ban_reason   | string    | Engel sebebi                     |
| last_login   | datetime  |                                  |
| created_at   | datetime  |                                  |
| updated_at   | datetime  |                                  |

### request_logs
| Kolon        | Tip       | Aciklama                    |
|--------------|-----------|-----------------------------|
| id           | uint (PK) | Auto increment              |
| user_id      | uint      | Index, users.id referansi   |
| firebase_uid | string    | Index, hizli kullanici sorgu|
| provider     | string    | Index (fal.ai, replicate..) |
| model        | string    |                             |
| prompt       | string    |                             |
| image_url    | string    | Orijinal gorsel URL'i       |
| result_url   | string    | Donusturulmus gorsel URL'i  |
| status       | string    | success / error             |
| duration_ms  | int64     | Islem suresi (ms)           |
| created_at   | datetime  |                             |

### provider_settings
| Kolon      | Tip       | Aciklama                    |
|------------|-----------|-----------------------------|
| id         | uint (PK) | Auto increment              |
| provider   | string    | Unique index                |
| api_key    | string    | (legacy, artik ENV'den)     |
| is_active  | bool      | Aktif/pasif durumu          |
| priority   | int       | Oncelik sirasi              |
| models     | text/JSON | Panelde tanimlanan model listesi |
| updated_at | datetime  |                             |

### categories
| Kolon       | Tip       | Aciklama                              |
|-------------|-----------|---------------------------------------|
| id          | uint (PK) | Auto increment                        |
| app_id      | string    | Index (default: "default")            |
| type        | string    | Index (photo \| video)                |
| slug        | string    | Composite unique: (app_id, type, slug)|
| name        | string    | Gosterim adi (cevirisi translation'da)|
| description | string    | Kisa aciklama (cevirisi translation'da)|
| icon_url    | string    | Kategori ikonu                        |
| is_popular  | bool      | Index, populer flag                   |
| is_trending | bool      | Index, trend flag                     |
| sort_order  | int       | Siralama                              |
| is_active   | bool      | Index, aktif/pasif                    |
| created_at  | datetime  |                                       |
| updated_at  | datetime  |                                       |

### templates
| Kolon               | Tip       | Aciklama                                    |
|---------------------|-----------|---------------------------------------------|
| id                  | uint (PK) | Auto increment                              |
| app_id              | string    | Index (default: "default")                  |
| slug                | string    | Unique index                                |
| name                | string    | Gosterim adi (cevirisi translation'da)      |
| description         | string    | Kisa aciklama (cevirisi translation'da)     |
| action_type         | string    | Index (image_generation/ai_chat/upscale/remove_bg/photo_restoration) |
| prompt              | string    | AI'a gonderilen prompt (CEVIRILMEZ)         |
| negative_prompt     | string    | Negatif prompt (opsiyonel)                  |
| provider            | string    | Kullanilacak provider (fal.ai vb.)          |
| model               | string    | Kullanilacak model (opsiyonel)              |
| category_id         | uint      | Index, categories.id FK                     |
| before_media_url    | string    | "Once" gorseli veya videosu (R2 CDN)        |
| before_media_type   | string    | image / video (default: image)              |
| after_media_url     | string    | "Sonra" AI demo gorseli/videosu (R2 CDN)    |
| after_media_type    | string    | image / video (default: image)              |
| reference_image_count | int     | 1 veya 2 (default: 1)                      |
| require_mom_photo   | bool      | Anne (mom) fotografini zorunlu kilar (default: false) |
| require_baby_photo  | bool      | Bebek (baby) fotografini zorunlu kilar (default: false) |
| require_dad_photo   | bool      | Baba (dad) fotografini zorunlu kilar (default: false) |
| hide_from_all       | bool      | Index, "Tumu" listesinden gizle             |
| aspect_ratio        | string    | Varsayilan en-boy orani (default: "1:1")   |
| supported_aspect_ratios | text  | Desteklenen oranlar, virgul ile ayrilmis (default: "1:1,4:5,9:16,16:9,3:4,4:3") |
| icon_url            | string    | Template ikonu                              |
| params              | text/JSON | Provider'a ozel parametreler                |
| credit_cost         | int       | Bu islem kac kredi (default: 1)             |
| is_active           | bool      | Index, aktif/pasif                          |
| is_featured         | bool      | Index, slider'da one cikarilacak mi         |
| is_premium          | bool      | Pro kullanicilar icin mi                    |
| sort_order          | int       | Siralama                                    |
| created_at          | datetime  |                                             |
| updated_at          | datetime  |                                             |

### slider_items
| Kolon       | Tip        | Aciklama                                |
|-------------|------------|-----------------------------------------|
| id          | uint (PK)  | Auto increment                          |
| app_id      | string     | Index                                   |
| type        | string     | Index (photo \| video)                  |
| template_id | uint       | Index, bagli template (opsiyonel)       |
| title       | string     | Baslik (cevirisi translation'da)        |
| description | string     | Aciklama (cevirisi translation'da)      |
| image_url   | string     | Slider arka plan gorseli (R2 CDN)       |
| frame_url   | string     | Seffaf PNG cerceve (ozel gun overlay)   |
| deep_link   | string     | Dokunulunca acilacak sayfa/template     |
| sort_order  | int        | Siralama                                |
| is_active   | bool       | Index, aktif/pasif                      |
| starts_at   | datetime*  | Gosterim baslangic tarihi (null=her zaman)|
| ends_at     | datetime*  | Gosterim bitis tarihi (null=her zaman)  |
| created_at  | datetime   |                                         |
| updated_at  | datetime   |                                         |

### quick_buttons
| Kolon       | Tip       | Aciklama                              |
|-------------|-----------|---------------------------------------|
| id          | uint (PK) | Auto increment                        |
| app_id      | string    | Index (default: "default")            |
| type        | string    | Index (photo \| video)                |
| title       | string    | Buton basligi (EN, cevirilmez)        |
| icon_url    | string    | Buton ikonu (PNG/SVG)                 |
| template_id | uint      | Index, bagli template                 |
| sort_order  | int       | Siralama                              |
| is_active   | bool      | Index, aktif/pasif                    |
| created_at  | datetime  |                                       |
| updated_at  | datetime  |                                       |

### onboarding_media
| Kolon         | Tip       | Aciklama                              |
|---------------|-----------|---------------------------------------|
| id            | uint (PK) | Auto increment                        |
| app_id        | string    | Index (default: "default")            |
| type          | string    | video / image                         |
| title         | string    | Baslik (cevirisi translation'da)      |
| description   | string    | Aciklama (cevirisi translation'da)    |
| media_url     | string    | Video/gorsel URL (R2 CDN)             |
| thumbnail_url | string    | Onizleme gorseli                      |
| sort_order    | int       | Siralama (0, 1, 2...)                 |
| is_active     | bool      | Index, aktif/pasif                    |
| created_at    | datetime  |                                       |
| updated_at    | datetime  |                                       |

### onboarding_reviews
| Kolon      | Tip       | Aciklama                              |
|------------|-----------|---------------------------------------|
| id         | uint (PK) | Auto increment                        |
| nickname   | string    | Kullanici adi (Sarah M.)              |
| photo_url  | string    | Yuvarlak profil fotosu URL (R2 CDN)   |
| review     | text      | Yorum metni (cevirisi translation'da) |
| rating     | int       | Puan 1-5 (default: 5)                 |
| sort_order | int       | Siralama (0, 1, 2...)                 |
| is_active  | bool      | Index, aktif/pasif                    |
| created_at | datetime  |                                       |
| updated_at | datetime  |                                       |

### translations
| Kolon       | Tip       | Aciklama                                        |
|-------------|-----------|-------------------------------------------------|
| id          | uint (PK) | Auto increment                                  |
| entity_type | string    | Index (template, category, slider, onboarding, review) |
| entity_id   | uint      | Index, ilgili kaydin ID'si                      |
| field       | string    | Cevrilen alan (name, description, title)        |
| language    | string    | Index, dil kodu (en, tr, de, fr...)             |
| value       | text      | Cevirilmis metin                                |
| created_at  | datetime  |                                                 |
| updated_at  | datetime  |                                                 |

### device_tokens
| Kolon        | Tip       | Aciklama                                        |
|--------------|-----------|-------------------------------------------------|
| id           | uint (PK) | Auto increment                                  |
| firebase_uid | string    | Index, token sahibi kullanicinin UID'i         |
| token        | string    | Unique index, FCM device token                  |
| platform     | string    | Index (ios \| android, default: ios)           |
| app_id       | string    | Index (default: "default")                      |
| locale       | string    | Cihaz dil kodu (en, tr, de...)                 |
| created_at   | datetime  |                                                 |
| updated_at   | datetime  |                                                 |

> Admin bildirim gonderirken gecersiz (unregistered/invalid) bulunan token'lar bu tablodan otomatik silinir.

### install_credit_claims
| Kolon              | Tip       | Aciklama                                         |
|--------------------|-----------|--------------------------------------------------|
| id                 | uint (PK) | Auto increment                                   |
| install_seed_hash  | string    | Unique index, `X-Install-Seed` SHA-256 hash'i   |
| first_firebase_uid | string    | Index, bu seed ile ilk kredi alan UID            |
| created_at         | datetime  |                                                  |
| updated_at         | datetime  |                                                  |

> Bu tablo sayesinde uygulama silinip tekrar yuklense bile ayni install seed ile baslangic kredisi yeniden verilmez.
