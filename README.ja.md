<p align="center">
<img alt="SiYuan" src="https://b3log.org/images/brand/siyuan-128.png">
</p>

<h1 align="center">SiYuan（fork）</h1>

<p align="center">
<a href="README.md">English</a>
| <a href="README.zh-CN.md">中文</a>
| <b>日本語</b>
| <a href="README.tr.md">Türkçe</a>
</p>

本リポジトリは [siyuan-note/siyuan](https://github.com/siyuan-note/siyuan) の個人 fork です。上流の stable リリースに追従し、自用の拡張を加えています。上流プロジェクト・公式リリース・ドキュメントは元リポジトリを参照してください。

## 上流との違い

上流 v3.8.10 stable ベース：

- **バックアップサブシステム** — 複数ターゲット（PostgreSQL / S3 / WebDAV）へのバックアップ。内容アドレス方式の重複排除スナップショット、スナップショット管理（参照カウントによる blob クリーンアップ付きの個別削除・全消去・zip エクスポート）、孤児スナップショットを防ぐ削除/編集ゲート、ターゲットごとの cron 予約バックアップ、「zip をローカルに保存」オフライン出口、復元後のデータインデックス自動再構築
- **メンテナンス読み取り専用ロック** — バックアップや復元の実行中、カーネルがワークスペースへの書き込みを拒否し、ロック解除までエディタは読み取り専用に切り替わる
- **リモートカーネル接続の拡張** — 履歴とアドレスごとの信頼設定を持つ接続マネージャー、平文 HTTP のリモートカーネルと plain HTTP での MCP OAuth、通常起動時の最後のリモートワークスペースへの自動再接続、ローカルワークスペースへの切り替え出口、ネイティブダイアログで保存されるリモートエクスポート
- **リモートワークスペースのオフライン閲覧** — 閲覧済みドキュメントはオフラインでも読める（ドキュメントキャッシュ、グレイ表示付きツリーキャッシュ、外部画像キャッシュ、キャッシュマネージャー、自動スヌーズとオフライン遅延を含むプラグインレジリエンス）
- **クエリ埋め込みブロックのテーブルモード** — クエリ埋め込みブロックを index / ファイル名 / ディレクトリ の 3 列のテーブルとしてレンダリングでき、raw SQL と最近表示（recent-viewed）の 2 種類のレンダリングモードに対応
- **PDF インラインプレビュー** — ドキュメント内でリンクされた PDF アセットを pdf.js でインラインプレビュー
- **コピーする可読パスにノートブック名を含める** — コピーされる可読パスにノートブック名が含まれる
- **fork ビルドパイプライン** — `v*` タグの push ごとに GitHub Actions が未署名（ad-hoc）の arm64 macOS DMG をビルドし、マルチアーキテクチャの Docker イメージを GHCR に公開

その他の機能は上流と同一で、fork は上流の stable リリースに追従して更新されます。バックアップサブシステムの設計・制約・動作は issue #15、リモートカーネル接続の拡張は issue #16 に文書化されています。

## インストール

- **macOS（Apple Silicon）**：[Releases](https://github.com/kamasylvia/siyuan/releases) から `siyuan-<version>-mac-arm64.dmg` をダウンロードし、マウント後に `SiYuan.app` を `/Applications` へコピー。ビルドは ad-hoc 未署名のため、初回起動を Gatekeeper にブロックされたら `xattr -dr com.apple.quarantine /Applications/SiYuan.app` を実行
- **Docker**：`ghcr.io/kamasylvia/siyuan` — `v` 接頭辞のバージョンタグと `latest`、amd64 + arm64

fork のフロントエンド機能は fork クライアントにのみ同梱されます。fork Docker イメージが提供するリモートワークスペースに接続する場合は fork クライアントを使用してください——公式クライアントに fork フロントエンドは含まれません。

## ビルド

`v*` タグの push で GitHub Actions が起動します：「Fork macOS Client」ワークフローが DMG（artifact + release）を生成し、「Docker Image (fork)」ワークフローが GHCR にイメージを push します。

## ライセンス

AGPL-3.0（上流プロジェクトのライセンスを継承）。
