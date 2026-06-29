// textpanel.go renderiza um painel de texto OpenGL para mostrar o status
// do servidor HTTP na tela nativa do app Android.
//
// Antes desta mudança, o app só mostrava um contador de FPS em fundo escuro
// — o usuário não tinha como saber se o servidor estava rodando nem qual
// URL acessar. Este painel resolve isso desenhando texto (fonte mono) numa
// textura GL que cobre toda a tela.
//
// Usa:
//   - golang.org/x/image/font/gofont/gomono  — fonte TTF embutida
//   - golang.org/x/image/font/opentype       — parser TTF + NewFace
//   - golang.org/x/mobile/exp/gl/glutil      — textura GL RGBA
package main

import (
        "image"
        "image/color"
        "image/draw"
        "log"

        "golang.org/x/image/font"
        "golang.org/x/image/font/gofont/gomono"
        "golang.org/x/image/font/opentype"
        "golang.org/x/image/math/fixed"
        "golang.org/x/mobile/event/size"
        "golang.org/x/mobile/exp/gl/glutil"
        "golang.org/x/mobile/geom"
)

// defaultFace é a fonte usada para todo o texto do painel. Inicializada
// em init() — é safe chamar opentype.Parse em init() porque não depende de GL.
var defaultFace font.Face

func init() {
        f, err := opentype.Parse(gomono.TTF)
        if err != nil {
                log.Fatalf("[textpanel] opentype.Parse(gomono): %v", err)
        }
        face, err := opentype.NewFace(f, &opentype.FaceOptions{
                Size:    18, // 18pt — legível em telas de 5-6"
                DPI:     72,
                Hinting: font.HintingFull,
        })
        if err != nil {
                log.Fatalf("[textpanel] opentype.NewFace: %v", err)
        }
        defaultFace = face
}

// cores usadas no painel (estilo Netflix dark)
var (
        colBackground = color.RGBA{0x14, 0x14, 0x14, 0xff} // #141414 fundo
        colTitle      = color.RGBA{0xe5, 0x09, 0x14, 0xff} // #e50914 vermelho Netflix
        colAccent     = color.RGBA{0x21, 0xa1, 0xeb, 0xff} // azul (URL)
        colOK         = color.RGBA{0x4c, 0xaf, 0x50, 0xff} // verde (running)
        colErr        = color.RGBA{0xef, 0x53, 0x50, 0xff} // vermelho (erro)
        colNormal     = color.RGBA{0xe0, 0xe0, 0xe0, 0xff} // cinza claro (corpo)
        colDim        = color.RGBA{0x80, 0x80, 0x80, 0xff} // cinza (hint)
)

// TextPanel renderiza uma lista de linhas de texto numa textura GL que
// é desenhada sobre a tela inteira.
type TextPanel struct {
        images *glutil.Images
        img    *glutil.Image
        lines  []textLine
        dirty  bool
}

// textLine é uma linha de texto com sua cor.
type textLine struct {
        text string
        col  color.Color
}

// NewTextPanel cria um painel com largura/altura em pixels. A textura
// interna é criada imediatamente (precisa de glctx já inicializado).
func NewTextPanel(images *glutil.Images, widthPx, heightPx int) *TextPanel {
        p := &TextPanel{
                images: images,
                dirty:  true,
        }
        // Cria a textura GL. O glutil.Image cuida do upload e draw.
        p.img = images.NewImage(widthPx, heightPx)
        return p
}

// SetLines atualiza as linhas do painel. Se as linhas forem iguais às
// atuais, não re-renderiza (evita re-upload desnecessário a cada frame).
func (p *TextPanel) SetLines(lines []textLine) {
        if linesEqual(p.lines, lines) {
                return
        }
        p.lines = lines
        p.dirty = true
}

// Draw renderiza (se dirty) e desenha a textura cobrindo a tela inteira.
// Deve ser chamado a cada paint.Event, na thread do GL.
func (p *TextPanel) Draw(sz size.Event) {
        if p.dirty {
                p.render()
        }
        if p.img == nil {
                return
        }
        // Desenha a textura esticada para cobrir a tela inteira.
        // Seguindo o padrão de debug.FPS: usa sz.WidthPt/sz.HeightPt (geom.Pt)
        // para os cantos, e p.img.RGBA.Bounds() como srcBounds.
        p.img.Draw(
                sz,
                geom.Point{X: 0, Y: 0},
                geom.Point{X: sz.WidthPt, Y: 0},
                geom.Point{X: 0, Y: sz.HeightPt},
                p.img.RGBA.Bounds(),
        )
}

// Release libera a textura GL. Chamar no lifecycle.CrossOff.
func (p *TextPanel) Release() {
        if p.img != nil {
                p.img.Release()
                p.img = nil
        }
}

// render redesenha o buffer RGBA e faz upload para a textura GL.
func (p *TextPanel) render() {
        rgba := p.img.RGBA
        // Fundo
        draw.Draw(rgba, rgba.Bounds(), image.NewUniform(colBackground), image.Point{}, draw.Src)

        drawer := &font.Drawer{
                Face: defaultFace,
                Dst:  rgba,
                // Src é definido por linha (cor varia)
                Dot: fixed.Point26_6{X: fixed.I(24), Y: fixed.I(48)},
        }

        for _, line := range p.lines {
                drawer.Src = image.NewUniform(line.col)
                drawer.Dot.X = fixed.I(24)
                drawer.DrawString(line.text)
                drawer.Dot.Y += fixed.I(28) // line height
        }

        p.img.Upload()
        p.dirty = false
}

// linesEqual compara duas listas de textLine.
func linesEqual(a, b []textLine) bool {
        if len(a) != len(b) {
                return false
        }
        for i := range a {
                if a[i].text != b[i].text {
                        return false
                }
                // Compara cor por componentes (RGBA)
                ar, ag, ab, aa := a[i].col.RGBA()
                br, bg, bb, ba := b[i].col.RGBA()
                if ar != br || ag != bg || ab != bb || aa != ba {
                        return false
                }
        }
        return true
}
