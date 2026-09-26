// The README's hero banner: docs/assets/hero.svg is its source and
// docs/assets/hero.png is exported from it.
//
//   swift scripts/hero/hero.swift compose   rewrites hero.svg: the art plus the
//                                           wordmark and tagline as outlines
//   swift scripts/hero/hero.swift render    exports hero.png from hero.svg
//
// `make hero` renders; `make hero-text` composes first.
//
// The text is outlines rather than <text> so it looks the same everywhere,
// with or without Inter installed. CoreText sets it, which is what gives it
// Inter's own kerning: a plain font reader sees only the old `kern` table,
// and Inter keeps its pairs in GPOS.
//
// Nothing here renders arbitrary SVG, and nothing on a stock Mac does from
// the command line. `render` understands exactly what `compose` writes — one
// <image>, and filled <path>s made of M, L, Q, C and Z — and refuses anything
// else by name rather than drawing something different.

import CoreGraphics
import CoreText
import Foundation
import ImageIO
import UniformTypeIdentifiers

// MARK: - The composition

// Laid out in the README's own pixels: it shows the banner 1200 wide, so the
// design's sizes (DESIGN-webflow.md) apply as written. The art keeps its full
// resolution underneath, and the export is the art's size.
let viewBox = CGSize(width: 1200, height: 600)
let art = "hero-art.png"

struct Line {
    let text: String
    let font: String  // PostScript name, checked before use
    let size: CGFloat
    let tracking: CGFloat
    let colour: String
    let note: String
}

// display-xxl, the hero headline: 80 px, 600, -0.8 px.
let wordmark = Line(
    text: "Lasso", font: "Inter-SemiBold", size: 80, tracking: -0.8,
    colour: "#ffffff", note: "Inter SemiBold 80px, -0.8px")
// body-lg, the lead paragraph: 28.8 px, 400, -0.288 px.
let tagline = Line(
    text: "Save video and audio from the web. No terminal.", font: "Inter-Regular", size: 28.8,
    tracking: -0.288, colour: "#ababab", note: "Inter Regular 28.8px, -0.288px")

// The rope crosses the middle of the art, from y 179 to 400 (measured on the
// art), so the text takes the clear band beneath it, centred there.
let band: ClosedRange<CGFloat> = 400...600
// From the wordmark's baseline to the tagline's capitals: the design's lg
// spacing plus a little, so the pair reads as one block rather than two.
let gap: CGFloat = 28

// MARK: - Compose

func fail(_ message: String) -> Never {
    FileHandle.standardError.write(("hero: " + message + "\n").data(using: .utf8)!)
    exit(1)
}

func font(_ line: Line) -> CTFont {
    let font = CTFontCreateWithName(line.font as CFString, line.size, nil)
    // CoreText substitutes silently when a font is missing, which would put
    // Helvetica in the banner and say nothing.
    guard CTFontCopyPostScriptName(font) as String == line.font else {
        fail("\(line.font) is not installed; get Inter from https://rsms.me/inter/")
    }
    return font
}

struct Typeset {
    let line: Line
    let ctLine: CTLine
    let width: CGFloat  // ink width: first glyph's left edge to last glyph's right
    let inkLeft: CGFloat
    let capHeight: CGFloat
}

func typeset(_ line: Line) -> Typeset {
    let f = font(line)
    let attributed = NSAttributedString(
        string: line.text,
        attributes: [
            NSAttributedString.Key(kCTFontAttributeName as String): f,
            NSAttributedString.Key(kCTKernAttributeName as String): line.tracking,
        ])
    let ct = CTLineCreateWithAttributedString(attributed)
    // Centred on the ink, not the advance, which carries the trailing
    // tracking and each glyph's side bearings.
    let ink = CTLineGetImageBounds(ct, nil)
    return Typeset(line: line, ctLine: ct, width: ink.width, inkLeft: ink.minX, capHeight: CTFontGetCapHeight(f))
}

/// The line's outlines as SVG path data, its baseline at y and centred on x.
func outline(_ s: Typeset, centreX: CGFloat, baseline: CGFloat) -> String {
    var d = ""
    let originX = centreX - s.width / 2 - s.inkLeft
    let runs = CTLineGetGlyphRuns(s.ctLine) as! [CTRun]
    for run in runs {
        let attrs = CTRunGetAttributes(run) as NSDictionary
        let runFont = attrs[kCTFontAttributeName as String] as! CTFont
        let count = CTRunGetGlyphCount(run)
        var glyphs = [CGGlyph](repeating: 0, count: count)
        var positions = [CGPoint](repeating: .zero, count: count)
        CTRunGetGlyphs(run, CFRange(location: 0, length: count), &glyphs)
        CTRunGetPositions(run, CFRange(location: 0, length: count), &positions)
        for (glyph, at) in zip(glyphs, positions) {
            guard let path = CTFontCreatePathForGlyph(runFont, glyph, nil) else { continue }
            // Glyphs are drawn y-up; the SVG is y-down.
            let x0 = originX + at.x
            let y0 = baseline - at.y
            func p(_ pt: CGPoint) -> String { num(x0 + pt.x) + " " + num(y0 - pt.y) }
            path.applyWithBlock { element in
                let e = element.pointee
                switch e.type {
                case .moveToPoint: d += "M" + p(e.points[0])
                case .addLineToPoint: d += "L" + p(e.points[0])
                case .addQuadCurveToPoint: d += "Q" + p(e.points[0]) + " " + p(e.points[1])
                case .addCurveToPoint:
                    d += "C" + p(e.points[0]) + " " + p(e.points[1]) + " " + p(e.points[2])
                case .closeSubpath: d += "Z"
                @unknown default: fail("an outline used a path element this script does not know")
                }
            }
        }
    }
    return d
}

func num(_ v: CGFloat) -> String {
    var s = String(format: "%.2f", Double(v))
    while s.hasSuffix("0") { s.removeLast() }
    if s.hasSuffix(".") { s.removeLast() }
    return s == "-0" ? "0" : s
}

func compose(svgPath: String) {
    let dir = URL(fileURLWithPath: svgPath).deletingLastPathComponent()
    let artURL = dir.appendingPathComponent(art)
    guard let source = CGImageSourceCreateWithURL(artURL as CFURL, nil),
        let image = CGImageSourceCreateImageAtIndex(source, 0, nil)
    else { fail("cannot read the art at \(artURL.path)") }

    let top = typeset(wordmark)
    let bottom = typeset(tagline)
    // The block's ink runs from the wordmark's capitals to the tagline's
    // baseline; the tagline has no descenders to allow for.
    let blockHeight = top.capHeight + gap + bottom.capHeight
    let blockTop = band.lowerBound + (band.upperBound - band.lowerBound - blockHeight) / 2
    let topBaseline = blockTop + top.capHeight
    let bottomBaseline = topBaseline + gap + bottom.capHeight
    let centre = viewBox.width / 2

    let svg = """
        <svg xmlns="http://www.w3.org/2000/svg" width="\(image.width)" height="\(image.height)" viewBox="0 0 \(num(viewBox.width)) \(num(viewBox.height))">
          <!-- Written by scripts/hero/hero.swift compose. Change it there and run make hero-text. -->
          <image href="\(art)" x="0" y="0" width="\(num(viewBox.width))" height="\(num(viewBox.height))" preserveAspectRatio="none"/>
          <!-- "\(wordmark.text)": \(wordmark.note) -->
          <path fill="\(wordmark.colour)" d="\(outline(top, centreX: centre, baseline: topBaseline))"/>
          <!-- "\(tagline.text)": \(tagline.note) -->
          <path fill="\(tagline.colour)" d="\(outline(bottom, centreX: centre, baseline: bottomBaseline))"/>
        </svg>

        """
    do {
        try svg.write(toFile: svgPath, atomically: true, encoding: .utf8)
    } catch {
        fail("cannot write \(svgPath): \(error)")
    }
    print("\(svgPath): wordmark baseline \(num(topBaseline)), tagline baseline \(num(bottomBaseline)) of \(num(viewBox.height))")
}

// MARK: - Render

final class Reader: NSObject, XMLParserDelegate {
    var size = CGSize.zero
    var viewBox = CGRect.zero
    var image: (href: String, rect: CGRect)?
    var paths: [(d: String, fill: String)] = []
    var problem: String?

    func parser(
        _ parser: XMLParser, didStartElement name: String, namespaceURI: String?,
        qualifiedName: String?, attributes a: [String: String]
    ) {
        func n(_ key: String) -> CGFloat { CGFloat(Double(a[key] ?? "") ?? 0) }
        switch name {
        case "svg":
            size = CGSize(width: n("width"), height: n("height"))
            let v = (a["viewBox"] ?? "").split(separator: " ").compactMap { Double($0) }
            viewBox = v.count == 4 ? CGRect(x: v[0], y: v[1], width: v[2], height: v[3]) : CGRect(origin: .zero, size: size)
        case "image":
            guard let href = a["href"] ?? a["xlink:href"] else { problem = "an <image> has no href"; return }
            image = (href, CGRect(x: n("x"), y: n("y"), width: n("width"), height: n("height")))
        case "path":
            guard let d = a["d"], let fill = a["fill"] else { problem = "a <path> needs d and fill"; return }
            paths.append((d, fill))
        default:
            problem = "<\(name)> is not something this renderer draws"
        }
    }
}

func colour(_ hex: String) -> CGColor {
    let h = hex.hasPrefix("#") ? String(hex.dropFirst()) : hex
    guard h.count == 6, let v = UInt32(h, radix: 16) else { fail("fill \(hex) is not #rrggbb") }
    return CGColor(
        srgbRed: CGFloat((v >> 16) & 0xff) / 255, green: CGFloat((v >> 8) & 0xff) / 255,
        blue: CGFloat(v & 0xff) / 255, alpha: 1)
}

func path(_ d: String) -> CGPath {
    let p = CGMutablePath()
    let scanner = Scanner(string: d)
    scanner.charactersToBeSkipped = CharacterSet(charactersIn: " ,\n\t")
    func pt() -> CGPoint {
        guard let x = scanner.scanDouble(), let y = scanner.scanDouble() else { fail("path data ended mid-point") }
        return CGPoint(x: x, y: y)
    }
    while !scanner.isAtEnd {
        guard let c = scanner.scanCharacter() else { break }
        switch c {
        case "M": p.move(to: pt())
        case "L": p.addLine(to: pt())
        case "Q":
            let c1 = pt()
            p.addQuadCurve(to: pt(), control: c1)
        case "C":
            let c1 = pt()
            let c2 = pt()
            p.addCurve(to: pt(), control1: c1, control2: c2)
        case "Z": p.closeSubpath()
        default: fail("path command \(c) is not one compose writes")
        }
    }
    return p
}

func render(svgPath: String, pngPath: String) {
    let svgURL = URL(fileURLWithPath: svgPath)
    guard let xml = XMLParser(contentsOf: svgURL) else { fail("cannot read \(svgPath)") }
    let reader = Reader()
    xml.delegate = reader
    guard xml.parse() else { fail("\(svgPath) is not well-formed: \(xml.parserError.map { "\($0)" } ?? "")") }
    if let problem = reader.problem { fail(problem) }
    guard reader.size.width > 0, reader.size.height > 0 else { fail("the <svg> needs a width and height") }

    let w = Int(reader.size.width)
    let h = Int(reader.size.height)
    let space = CGColorSpace(name: CGColorSpace.sRGB)!
    guard
        let ctx = CGContext(
            data: nil, width: w, height: h, bitsPerComponent: 8, bytesPerRow: 0, space: space,
            bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)
    else { fail("cannot make a \(w)x\(h) canvas") }
    ctx.interpolationQuality = .high
    ctx.setShouldAntialias(true)

    // SVG is y-down in viewBox units; CoreGraphics is y-up in pixels.
    ctx.translateBy(x: 0, y: CGFloat(h))
    ctx.scaleBy(x: CGFloat(w) / reader.viewBox.width, y: -CGFloat(h) / reader.viewBox.height)
    ctx.translateBy(x: -reader.viewBox.minX, y: -reader.viewBox.minY)

    if let (href, rect) = reader.image {
        let url = svgURL.deletingLastPathComponent().appendingPathComponent(href)
        guard let source = CGImageSourceCreateWithURL(url as CFURL, nil),
            let image = CGImageSourceCreateImageAtIndex(source, 0, nil)
        else { fail("cannot read \(url.path)") }
        ctx.saveGState()
        // Images draw y-up too: flip within the image's own box.
        ctx.translateBy(x: rect.minX, y: rect.maxY)
        ctx.scaleBy(x: 1, y: -1)
        ctx.draw(image, in: CGRect(origin: .zero, size: rect.size))
        ctx.restoreGState()
    }
    for (d, fill) in reader.paths {
        ctx.addPath(path(d))
        ctx.setFillColor(colour(fill))
        ctx.fillPath()
    }

    guard let out = ctx.makeImage(),
        let dest = CGImageDestinationCreateWithURL(
            URL(fileURLWithPath: pngPath) as CFURL, UTType.png.identifier as CFString, 1, nil)
    else { fail("cannot write \(pngPath)") }
    CGImageDestinationAddImage(dest, out, nil)
    guard CGImageDestinationFinalize(dest) else { fail("cannot write \(pngPath)") }
    print("\(pngPath): \(w)x\(h), exported from \(svgPath)")
}

// MARK: - Main

let args = CommandLine.arguments.dropFirst()
switch args.first {
case "compose":
    compose(svgPath: args.dropFirst().first ?? "docs/assets/hero.svg")
case "render":
    let rest = Array(args.dropFirst())
    render(svgPath: rest.first ?? "docs/assets/hero.svg", pngPath: rest.dropFirst().first ?? "docs/assets/hero.png")
default:
    fail("usage: hero.swift compose [hero.svg] | render [hero.svg] [hero.png]")
}
