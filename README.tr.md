<p align="center">
<img alt="SiYuan" src="https://b3log.org/images/brand/siyuan-128.png">
</p>

<h1 align="center">SiYuan (fork)</h1>

<p align="center">
<a href="README.md">English</a>
| <a href="README.zh-CN.md">中文</a>
| <a href="README.ja.md">日本語</a>
| <b>Türkçe</b>
</p>

Bu depo, [siyuan-note/siyuan](https://github.com/siyuan-note/siyuan) projesinin kişisel bir fork'udur. Upstream stable sürümlerini takip eder ve kendi kullanımı için geliştirmeler ekler. Upstream proje, resmi sürümler ve belgeler için orijinal depoya bakın.

## Upstream'den farklar

Upstream v3.8.10 stable tabanlıdır:

- **Yedekleme alt sistemi** — çok hedefli yedekleme (PostgreSQL, S3, WebDAV), içerik adresli ve tekrarsız anlık görüntüler, anlık görüntü yönetimi (referans sayımlı blob temizliğiyle tekil anlık görüntü silme, tümünü temizleme, zip dışa aktarma), yetim anlık görüntüleri önleyen silme ve düzenleme kapıları, hedef başına cron zamanlamalı yedekleme, çevrimdışı "Zip'i yerelde sakla" çıkışı ve otomatik veri indeksi yeniden oluşturma ile geri yükleme
- **Bakım salt okunur kilidi** — yedekleme veya geri yükleme sürerken çekirdek çalışma alanı yazmalarını reddeder ve kilit serbest bırakılana kadar düzenleyiciler salt okunur moda geçer
- **Uzak çekirdek bağlantı geliştirmeleri** — geçmiş ve adres başına güven içeren bağlantı yöneticisi, düz HTTP uzak çekirdekler ve plain HTTP üzerinden MCP OAuth, normal açılışta son uzak çalışma alanına otomatik yeniden bağlanma, yerel çalışma alanına geçiş çıkışı ve yerel iletişim kutusuyla kaydedilen uzak dışa aktarmalar
- **Uzak çalışma alanları için çevrimdışı salt okunur** — görüntülenen belgeler çevrimdışı da okunabilir kalır (belge önbelleği, gri tonlamalı ağaç önbelleği, dış görsel önbelleği, önbellek yöneticisi, otomatik uyku ve çevrimdışı erteleme içeren eklenti dayanıklılığı)
- **Sorgu gömme bloğu tablo modu** — sorgu gömme blokları dizin / dosya adı / klasör sütunlarından oluşan bir tablo olarak görüntülenebilir; ham SQL ve son görüntülenenler (recent-viewed) modlarını destekler
- **Satır içi PDF önizleme** — belgelerde bağlanan PDF varlıkları pdf.js ile satır içi önizlenir
- **Kopyalanan okunabilir yola defter adını dahil et** — kopyalanan okunabilir yol defter adını içerir
- **fork derleme hattı** — her `v*` etiketi, GitHub Actions ile imzasız (ad-hoc) arm64 macOS DMG derler ve çok mimarili Docker imajlarını GHCR'a yayınlar

Diğer tüm işlevler upstream ile aynıdır; fork, upstream stable sürümlerini takip ederek güncellenir. Yedekleme alt sisteminin tasarımı, kısıtları ve davranışı issue #15'te; uzak çekirdek bağlantı geliştirmeleri issue #16'te belgelenmiştir.

## Kurulum

- **macOS (Apple Silicon)**: [Releases](https://github.com/kamasylvia/siyuan/releases) sayfasından `siyuan-<version>-mac-arm64.dmg` dosyasını indirin, bağlayın ve `SiYuan.app`'i `/Applications` içine kopyalayın. Derleme ad-hoc imzasızdır; Gatekeeper ilk açılışı engellerse `xattr -dr com.apple.quarantine /Applications/SiYuan.app` komutunu çalıştırın
- **Docker**: `ghcr.io/kamasylvia/siyuan` — `v` önekli sürüm etiketleri ve `latest`, amd64 + arm64

fork ön yüz özellikleri yalnızca fork istemcisiyle gelir. fork Docker imajının sunduğu uzak çalışma alanına bağlanırken fork istemcisini kullanın; resmi istemci asla fork ön yüzünü içermez.

## Derleme

`v*` etiketi push'landığında GitHub Actions tetiklenir: "Fork macOS Client" iş akışı DMG'yi (artifact ve release) üretir, "Docker Image (fork)" iş akışı imajı GHCR'a gönderir.

## Lisans

AGPL-3.0, upstream proje lisansı devralınmıştır.
