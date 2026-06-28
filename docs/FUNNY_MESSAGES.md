# Mensagens engraçadas da API 🤣

Coletadas durante a engenharia reversa do APK e da API ao vivo. Algumas são typos, outras são mensagens engraçadas em PT-BR que aparecem no código nativo ou nas respostas HTTP.

---

## 🛑 Emoji no log nativo

```kotlin
// MainActivity.kt, configureFlutterEngine$lambda$0
System.out.println((Object) ("URL recebida no nativo 🛑: " + str));
```

Quando o Flutter envia a URL do hoster para o Kotlin via `MethodChannel`, o dev colocou um 🛑 (sinal de parada) no log.

---

## 💙 Anti-emulador com coração azul

```kotlin
// MainActivity.kt, configureFlutterEngine
isEmulator = MainActivityKt.isEmulator();
if (isEmulator) {
    System.out.print((Object) "emulator 💙");
    throw new IllegalStateException();
}
```

Se detectar emulador, loga `emulator 💙` e **crasha a app**. O coração azul provavelmente é uma piada interna — talvez "blue screen" ou apenas um toque pessoal.

---

## 📝 Typo "rNao" no log de erro

```java
// ExtractorLinks.java, linha 98
DriverManager.println("URL rNao encontrada 🛑");
```

Quando nenhum extractor match a URL, loga `URL rNao encontrada 🛑`. O typo é em "não" — provavelmente o dev esqueceu o acento e juntou tudo. Usa `DriverManager.println()` (método de JDBC!) só para fazer log, o que é estranho.

---

## 🎬 "Steam Video" em vez de "Stream Video"

```html
<!-- Resposta de /embed-api/?action=embed&url=... -->
<!DOCTYPE html>
<html lang="pt-br">
<head>
  <title>Steam Video</title>  <!-- deveria ser "Stream" -->
  ...
</head>
```

O título HTML retornado pelo endpoint `/embed-api/?action=embed&url=...` é "Steam Video" (sim, como a plataforma de jogos da Valve) — typo de "Stream Video".

---

## 🏆 "Esee é o Top 1" — typo em "Esse"

```html
<server-selector data-server="67sZjX-555wzuE...">
  <span>Esee é o Top 1, rápido e poucos anúncios!</span>
</server-selector>
```

Descrição do 3º `<server-selector>` no embed: "Esee é o Top 1, rápido e poucos anúncios!". Falta um "s" — deveria ser "Esse".

---

## 🚀 "O 2º melhor muito rápido!"

```html
<server-selector data-server="X69yzq76PTAszt1...">
  <span>O 2º melhor muito rápido!</span>
</server-selector>
```

Descrição do 4º `<server-selector>`: "O 2º melhor muito rápido!". Mas se ele é o 4º, como pode ser o 2º melhor? Talvez os dois primeiros sejam idênticos em qualidade e este seja o "2º" na ordem de preferência real.

---

## 📢 "Velocidade ok e poucos anúncios"

```html
<server-selector data-server="pq5XG9_s-NStknp...">
  <span>Velocidade ok e poucos anúncios</span>
</server-selector>
<server-selector data-server="uddfNVBR2hO4SF...">
  <span>Velocidade ok e poucos anúncios</span>
</server-selector>
```

Os dois primeiros servers compartilham a mesma descrição. Ok mas não animadora.

---

## 📋 "Preencha os dados necessários"

```bash
curl https://supercine-tv.net/wp-json/api/add
# {"response":false,"message":"Preencha os dados necess\u00e1rios"}
```

Resposta padrão do endpoint `/api/add` quando faltam parâmetros. Funcional, mas genérica — não diz **quais** dados são necessários.

---

## 🚫 "site não suportado"

```bash
curl 'https://supercine-tv.net/wp-json/site/extractor?url=https://doodstream.com/e/abc'
# {"status":"error","message":"site n\u00e3o suportado"}
```

O endpoint server-side de extractor não suporta **nenhum** dos 8 hosters que o APK usa client-side. Curioso.

---

## 🤷 "falta dados"

```bash
curl https://supercine-tv.net/wp-json/site/extractor
# {"status":"error","message":"falta dados"}
```

Mensagem direta, sem cerimônia. Estilo de quem fala: "falta dados, parça".

---

## 🤔 "Ação desconhecida"

```bash
curl -X POST https://supercine-tv.net/wp-json/inbox/report -d '{}'
# {"success":false,"message":"A\u00e7\u00e3o desconhecida"}
```

Endpoint de report pede uma "action" mas não documenta quais são válidas. Ação desconhecida mesmo.

---

## 🔐 "Código e device são obrigatórios"

```bash
curl -X POST https://supercine-tv.net/wp-json/auth/login -d '{}'
# {"success":false,"premium":false,"error":"C\u00f3digo e device s\u00e3o obrigat\u00f3rios"}
```

Login exige `code` (código de ativação) e `device` (identificador do aparelho). Mensagem correta e clara.

---

## ❌ "Código inválido"

```bash
curl -X POST https://supercine-tv.net/wp-json/auth/login -d '{"code":"INVALID","device":"test"}'
# {"success":false,"premium":false,"error":"C\u00f3digo inv\u00e1lido"}
```

Não diz se o código não existe, se já foi usado, ou se expirou — apenas "inválido".

---

## 🔍 "Pedido não encontrado"

```bash
curl -X POST https://supercine-tv.net/wp-json/auth/checkout-status -d '{"pix_id":"x","device":"y"}'
# {"success":false,"premium":false,"error":"Pedido n\u00e3o encontrado"}
```

---

## 📱 "Device é obrigatório"

```bash
curl -X POST https://supercine-tv.net/wp-json/auth/history -d '{}'
# {"success":false,"premium":false,"error":"Device \u00e9 obrigat\u00f3rio"}
```

---

## 💬 "possivel fazer pedidos em tempo real com o suporte !!"

Encontrada em `strings libapp.so`:

```
possivel fazer pedidos em tempo real com o suporte !!
```

Provavelmente parte de um texto de UI explicando que premium users podem pedir filmes/séries em tempo real. Faltou um "é" antes do "possivel" e um acento.

---

## 🎥 "Erro ao abrir player nativo: "

```
Erro ao abrir player nativo:
```

Mensagem de erro quando o `NativeView` (VLC/ExoPlayer) falha ao inicializar. O `:` no fim sugere que ela é concatenada com a mensagem de erro técnica.

---

## 📺 "um aplicativo para ajudar amantes de streaming de v"

```
 um aplicativo para ajudar amantes de streaming de v
```

Aparece truncada em `strings libapp.so`. Provavelmente continua com `ídeos` em outra string (Dart AOT às vezes quebra strings longas em múltiplas entradas).

---

## 📦 "Supercine Tv V1.0"

```
Supercine Tv V1.0
```

String de versão curta usada em algum lugar da UI. Sem NT extremamente formal — usa "Tv" em vez de "TV".

---

## 🎯 "O Supercine.tv "

```
O Supercine.tv 
```

Note o espaço no fim. Provavelmente parte de uma frase que continua em outra string.

---

## 🤖 Path de build do dev

```
file:///Users/ianoliveira/Documents/GitHub/supercine_app/.dart_tool/flutter_build/dart_plugin_registrant.dart
```

O path de build do desenvolvedor (username `ianoliveira`) vazou no binário porque o Dart AOT mantém referências a arquivos fonte para stack traces. Não é exatamente engraçado, mas é uma exposição acidental de informação pessoal.

---

## Resumo

| Categoria | Exemplo | Nível de engrace |
|---|---|---|
| Emoji em log | `URL recebida no nativo 🛑` | 🤣🤣 |
| Anti-emulador poético | `emulator 💙` | 🤣🤣🤣 |
| Typo em log | `URL rNao encontrada` | 🤣 |
| Typo em título HTML | `<title>Steam Video</title>` | 🤣🤣🤣 |
| Typo em descrição | `Esee é o Top 1` | 🤣🤣🤣 |
| Lógica estranha | `O 2º melhor muito rápido!` (4º server) | 🤣🤣 |
| JDBC para log | `DriverManager.println(...)` | 🤣🤣 |
| Vazamento de path | `file:///Users/ianoliveira/...` | 😬 |
| Descrição truncada | `...amantes de streaming de v` | 🤣 |

No total: typos e decisões estranhas suficientes para um sorriso. Nada crtítico para o usuário final, mas mostra que o código não passou por uma revisão de strings muito rigorosa. 😄
