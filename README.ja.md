# Local Figma MCP Bridge

[English](./README.md) | 日本語

Figma Plugin とローカル MCP サーバを経由して、現在開いている Figma ファイルを読み取るための非公式ローカル MCP ブリッジです。

Local Figma MCP Bridge は、意図的に小さく、ローカルファーストで、読み取り専用に保っています。現在の Figma 選択範囲を確認し、ローカル開発のために書き出すことに特化しています。

このプロジェクトはローカル開発と実験を目的としています。Figma とは提携しておらず、Figma による承認・後援を受けたものではありません。

```text
MCP client
  -> local MCP server (stdio)
  -> WebSocket bridge (localhost:8787)
  -> Figma Plugin UI
  -> Figma Plugin main thread
  -> currently open Figma file
```

## できること

- Figma Plugin がローカル MCP サーバに接続されているか確認する
- 開いている Figma ファイルと現在ページの基本情報を取得する
- 選択中ノードの構造、サイズ、位置、色、レイアウトメタデータ、子ノードを要約する
- 選択中ノードを `SVG` または `PNG` として書き出す

## 位置づけ

このプロジェクトは、意図的に小さく、読み取り専用に保っています。

複数ファイル操作や AI による Figma 編集まで扱う広い Figma MCP ブリッジとは異なり、Local Figma MCP Bridge は「現在選択している Figma ノードを確認し、必要に応じて `SVG` / `PNG` として書き出す」ことに特化しています。実装やレビューのためにデザイン情報だけを参照したい場合に、挙動を理解・監査しやすい構成です。

### やらないこと

- MCP client から Figma ファイルを編集すること
- Figma ノードの作成、削除、グループ化、移動
- 複数の Figma ファイル接続を管理すること
- 現在の選択範囲だけで足りる場面で、ファイル全体をデフォルトで読み取ること

## プライバシーとデータの扱い

このツールは、Plugin が実行されている Figma ファイル内のデザインデータにアクセスできます。

Figma からローカル MCP サーバへ送信される可能性があるデータは次のとおりです。

- ルートファイル名、ページ名、ページ ID、Figma file key などのファイルメタデータ
- ノード ID、名前、寸法、位置、色、エフェクト、レイアウトプロパティ、子階層などの選択中ノードのメタデータ
- base64 エンコードされた `SVG` または `PNG` の選択範囲書き出しデータ。この中には画像、テキスト、アイコン、製品 UI、顧客データ、その他の機密デザイン情報が含まれる可能性があります。

デフォルト実装では、このデータは `ws://localhost:8787` のローカル WebSocket サーバにのみ送信されます。このプロジェクト自体はリモートサービスへデータを送信せず、Plugin manifest でも本番用の外部ドメインへのネットワークアクセスは許可していません。

非公開のデザインファイルで使う場合は、ファイル所有者または所属組織から必要な許可を得てください。明示的な許可なく、書き出したデザインアセット、顧客制作物、内部 UI、ファイル由来のサンプルデータを公開しないでください。

## セキュリティ上の注意

- このブリッジはローカル開発用です。`FIGMA_BRIDGE_HOST` や `8787` ポートを public network に公開しないでください。
- このブリッジに接続した MCP client は、選択中の Figma ノードメタデータや書き出しデータを要求できるものとして扱ってください。
- このプロジェクトは write tool を提供しません。現在の選択範囲の確認と書き出しのみを目的としています。
- MCP サーバは現在 WebSocket client を認証していません。信頼できるマシン上でのみ実行し、独自に認証や transport protection を追加していない限り host は `localhost` に固定してください。
- 実際の Figma 書き出しアセット、`.env` ファイル、access token、ログ、生成された scratch output を commit しないでください。

## セットアップ

```bash
npm install
npm run build
```

## Figma Plugin の読み込み

1. Figma Desktop を開く
2. `Plugins > Development > Import plugin from manifest...` を選ぶ
3. `packages/figma-plugin/dist/manifest.json` を選択する
4. `Local Figma MCP Bridge` を実行する
5. ローカル MCP サーバが起動していると、Plugin UI が自動的に接続します

UI が `Disconnected` と表示される場合は、ローカル MCP サーバを起動して Plugin の自動再接続を待ってください。

```bash
npm run dev:mcp
```

## MCP Client 設定

stdio server をサポートする MCP client であれば、このブリッジを利用できます。ビルド済み MCP サーバを MCP client の設定に追加してください。以下は Codex での設定例です。

```json
{
  "mcpServers": {
    "local-figma": {
      "command": "node",
      "args": [
        "/absolute/path/to/packages/mcp-server/dist/index.js"
      ],
      "env": {
        "FIGMA_BRIDGE_PORT": "8787"
      }
    }
  }
}
```

任意の環境変数:

- `FIGMA_BRIDGE_HOST`: WebSocket host。デフォルトは `localhost`
- `FIGMA_BRIDGE_PORT`: WebSocket port。デフォルトは `8787`

## MCP Tools

- `figma_status`: Figma Plugin が接続されているか確認する
- `figma_file_info`: 開いている Figma ファイルと現在ページの基本情報を取得する
- `figma_get_selection`: 選択中の Figma ノードの JSON 要約を返す。任意の `depth` パラメータの上限は `5`
- `figma_export_selection`: 現在の Figma 選択範囲を base64 エンコードされた `SVG` または `PNG` として書き出す

## Figma Manifest のネットワークアクセス

Plugin manifest では次の設定を使っています。

```json
{
  "networkAccess": {
    "allowedDomains": ["none"],
    "devAllowedDomains": [
      "http://localhost:8787",
      "ws://localhost:8787"
    ]
  }
}
```

これにより、本番用のネットワークアクセスを無効にしたまま、ローカル開発用ブリッジだけを許可しています。このプロジェクトを変更してリモートサービスにデータを送信する場合は、manifest、ドキュメント、privacy policy、ユーザー同意フローを適切に更新してください。
