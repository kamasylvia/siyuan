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

Upstream v3.8.8 stable tabanlıdır:

- **Sorgu gömme bloğu tablo modu** — sorgu gömme blokları dizin / dosya adı / klasör sütunlarından oluşan bir tablo olarak görüntülenebilir; ham SQL ve son görüntülenenler (recent-viewed) modlarını destekler
- **Belge ağacı sayfalama** — ilk ve son sayfa düğmeli sayfalama kontrolleri
- **Satır içi PDF önizleme** — belgelerde bağlanan PDF varlıkları pdf.js ile satır içi önizlenir
- **Düz HTTP uzak çekirdekler** — düz HTTP uzak çekirdek bağlantılarına ve MCP OAuth uç noktalarına izin verilir
- **fork derleme hattı** — her `v*` etiketi, GitHub Actions ile imzasız (ad-hoc) arm64 macOS DMG derler ve çok mimarili Docker imajlarını GHCR'a yayınlar

Diğer tüm işlevler upstream ile aynıdır; fork, upstream stable sürümlerini takip ederek güncellenir.

## Kurulum

- **macOS (Apple Silicon)**: [Releases](https://github.com/kamasylvia/siyuan/releases) sayfasından `siyuan-<version>-mac-arm64.dmg` dosyasını indirin, bağlayın ve `SiYuan.app`'i `/Applications` içine kopyalayın. Derleme ad-hoc imzasızdır; Gatekeeper ilk açılışı engellerse `xattr -dr com.apple.quarantine /Applications/SiYuan.app` komutunu çalıştırın
- **Docker**: `ghcr.io/kamasylvia/siyuan` — `v` önekli sürüm etiketleri ve `latest`, amd64 + arm64

fork ön yüz özellikleri yalnızca fork istemcisiyle gelir. fork Docker imajının sunduğu uzak çalışma alanına bağlanırken fork istemcisini kullanın; resmi istemci asla fork ön yüzünü içermez.

## Derleme

`v*` etiketi push'landığında GitHub Actions tetiklenir: "Fork macOS Client" iş akışı DMG'yi (artifact ve release) üretir, "Docker Image (fork)" iş akışı imajı GHCR'a gönderir.

## Lisans

AGPL-3.0, upstream proje lisansı devralınmıştır.
