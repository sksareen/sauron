// SauronCapture — screenshot → annotate → route to Claude Code
// Global hotkey: Ctrl+Shift+S
// Features: freehand drawing, text annotations, screenshot gallery
// Build: swiftc -O -o SauronCapture -framework Cocoa -framework Carbon main.swift

import Cocoa
import Carbon.HIToolbox

// ─── Constants ───────────────────────────────────────────────────────────────

let SAURON_DIR = FileManager.default.homeDirectoryForCurrentUser
    .appendingPathComponent(".sauron")
let SCREENSHOTS_DIR = SAURON_DIR.appendingPathComponent("screenshots")
let LINE_WIDTH: CGFloat = 3.0
let TEXT_FONT_SIZE: CGFloat = 16.0
let GALLERY_THUMB_SIZE: CGFloat = 180.0

// ─── Entry Point ─────────────────────────────────────────────────────────────

let app = NSApplication.shared
app.setActivationPolicy(.accessory)
let appDelegate = CaptureAppDelegate()
app.delegate = appDelegate
app.run()

// ─── App Delegate ────────────────────────────────────────────────────────────

class CaptureAppDelegate: NSObject, NSApplicationDelegate {
    var statusItem: NSStatusItem!
    var previousApp: NSRunningApplication?
    var annotationWindow: AnnotationWindow?
    var galleryWindow: GalleryWindow?
    var isCapturing = false
    var globalMonitor: Any?
    var localMonitor: Any?
    var hotkeyRef: EventHotKeyRef?

    func applicationDidFinishLaunching(_ notification: Notification) {
        setupMenuBar()
        registerHotkey()
        NSLog("SauronCapture: launched — hotkey ⌃⇧S, library via ◎ menu")
    }

    func setupMenuBar() {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        statusItem.button?.title = "◎"

        let menu = NSMenu()
        menu.addItem(withTitle: "Capture  ⌃⇧S", action: #selector(menuCapture), keyEquivalent: "")
        menu.addItem(withTitle: "Library", action: #selector(showLibrary), keyEquivalent: "")
        menu.addItem(.separator())
        menu.addItem(withTitle: "Quit", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        statusItem.menu = menu
    }

    @objc func menuCapture() { startCapture() }

    @objc func showLibrary() {
        if let existing = galleryWindow, existing.isVisible {
            existing.makeKeyAndOrderFront(nil)
            NSApp.activate()
            return
        }
        let win = GalleryWindow()
        galleryWindow = win
        win.makeKeyAndOrderFront(nil)
        NSApp.activate()
    }

    func registerHotkey() {
        if AXIsProcessTrusted() {
            installGlobalMonitor()
        } else {
            // Only prompt once when not yet trusted
            AXIsProcessTrustedWithOptions(
                [kAXTrustedCheckOptionPrompt.takeUnretainedValue(): true] as CFDictionary)
            NSLog("SauronCapture: ⚠️ Accessibility not granted — system prompt shown. Polling...")
            pollForAccessibility()
        }
    }

    func pollForAccessibility() {
        Timer.scheduledTimer(withTimeInterval: 2.0, repeats: true) { [weak self] timer in
            if AXIsProcessTrusted() {
                timer.invalidate()
                NSLog("SauronCapture: ✅ Accessibility granted")
                self?.installGlobalMonitor()
            }
        }
    }

    func installGlobalMonitor() {
        globalMonitor = NSEvent.addGlobalMonitorForEvents(matching: .keyDown) { [weak self] event in
            if self?.isHotkey(event) == true {
                DispatchQueue.main.async { self?.startCapture() }
            }
        }
        localMonitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { [weak self] event in
            if self?.isHotkey(event) == true {
                DispatchQueue.main.async { self?.startCapture() }
                return nil
            }
            return event
        }

        if globalMonitor != nil {
            NSLog("SauronCapture: ✅ Global hotkey ⌃⇧S active")
        } else {
            NSLog("SauronCapture: ⚠️ Global monitor failed — check Accessibility permission")
        }
    }

    func isHotkey(_ event: NSEvent) -> Bool {
        let flags = event.modifierFlags.intersection(.deviceIndependentFlagsMask)
        return event.keyCode == 1
            && flags.contains(.control)
            && flags.contains(.shift)
            && !flags.contains(.command)
            && !flags.contains(.option)
    }

    func startCapture() {
        guard !isCapturing else { return }
        isCapturing = true

        if let existing = annotationWindow {
            existing.orderOut(nil)
            annotationWindow = nil
        }

        previousApp = NSWorkspace.shared.frontmostApplication

        DispatchQueue.global(qos: .userInitiated).async { [self] in
            let ts = Int(Date().timeIntervalSince1970 * 1000)
            let tempPath = "/tmp/sauron_cap_\(ts).png"

            let task = Process()
            task.executableURL = URL(fileURLWithPath: "/usr/sbin/screencapture")
            task.arguments = ["-i", "-x", "-t", "png", tempPath]

            do { try task.run() } catch {
                NSLog("SauronCapture: screencapture error: \(error)")
                DispatchQueue.main.async { self.isCapturing = false }
                return
            }
            task.waitUntilExit()

            guard task.terminationStatus == 0,
                  FileManager.default.fileExists(atPath: tempPath),
                  let image = NSImage(contentsOfFile: tempPath) else {
                DispatchQueue.main.async { self.isCapturing = false }
                return
            }

            DispatchQueue.main.async { [self] in
                let win = AnnotationWindow(
                    image: image, tempPath: tempPath, previousApp: previousApp)
                win.onDismiss = { [weak self] in
                    self?.isCapturing = false
                    self?.annotationWindow = nil
                }
                annotationWindow = win
                win.center()
                win.makeKeyAndOrderFront(nil)
                NSApp.activate()
                win.makeFirstResponder(win.canvas)
                win.toggleTextMode()
                // Re-assert first responder after activation settles
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {
                    win.makeFirstResponder(win.canvas)
                }
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.3) {
                    win.makeFirstResponder(win.canvas)
                }
            }
        }
    }
}

// ─── Text Annotation Model ──────────────────────────────────────────────────

struct TextAnnotation {
    let text: String
    let point: NSPoint
    let color: NSColor
    let fontSize: CGFloat
}

// ─── Annotation Window ──────────────────────────────────────────────────────

class AnnotationWindow: NSPanel {
    let canvas: AnnotationView
    let tempPath: String
    let previousApp: NSRunningApplication?
    var onDismiss: (() -> Void)?
    private var dismissed = false
    var textModeButton: NSButton!
    var isTextMode = false

    init(image: NSImage, tempPath: String, previousApp: NSRunningApplication?) {
        self.tempPath = tempPath
        self.previousApp = previousApp

        let screen = NSScreen.main!.visibleFrame
        let maxW = screen.width * 0.8
        let maxH = screen.height * 0.85
        let scale = min(maxW / image.size.width, maxH / image.size.height, 1.0)
        let imgW = image.size.width * scale
        let imgH = image.size.height * scale
        let barH: CGFloat = 44

        let frame = NSRect(
            x: screen.midX - imgW / 2,
            y: screen.midY - (imgH + barH) / 2,
            width: imgW,
            height: imgH + barH)

        canvas = AnnotationView(
            frame: NSRect(x: 0, y: barH, width: imgW, height: imgH))
        canvas.image = image

        super.init(
            contentRect: frame,
            styleMask: [.titled, .closable, .miniaturizable, .resizable],
            backing: .buffered, defer: false)

        title = "Annotate — ⌘Enter Send · Esc Cancel"
        level = .floating
        isReleasedWhenClosed = false
        hidesOnDeactivate = false
        collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]

        let container = NSView(frame: NSRect(x: 0, y: 0, width: imgW, height: imgH + barH))
        container.autoresizingMask = [.width, .height]
        container.addSubview(canvas)
        container.addSubview(buildToolbar(width: imgW, height: barH))
        contentView = container

        // Enable mouse tracking on canvas for live text cursor
        let area = NSTrackingArea(
            rect: .zero,
            options: [.mouseMoved, .activeAlways, .inVisibleRect],
            owner: canvas, userInfo: nil)
        canvas.addTrackingArea(area)
    }

    // MARK: - Toolbar

    func buildToolbar(width: CGFloat, height: CGFloat) -> NSView {
        let bar = NSView(frame: NSRect(x: 0, y: 0, width: width, height: height))
        bar.wantsLayer = true
        bar.layer?.backgroundColor = NSColor(white: 0.12, alpha: 1).cgColor

        var x: CGFloat = 8

        // Clear
        let clearBtn = makeButton("Clear", action: #selector(clearAll))
        clearBtn.frame = NSRect(x: x, y: 8, width: 52, height: 28)
        bar.addSubview(clearBtn)
        x += 56

        // Undo
        let undoBtn = makeButton("Undo", action: #selector(undoLast))
        undoBtn.frame = NSRect(x: x, y: 8, width: 52, height: 28)
        bar.addSubview(undoBtn)
        x += 60

        // Separator
        let sep1 = NSBox(frame: NSRect(x: x, y: 6, width: 1, height: 28))
        sep1.boxType = .separator
        bar.addSubview(sep1)
        x += 8

        // Color dots
        let colors: [(NSColor, String)] = [
            (.systemRed, "Red"), (.systemYellow, "Yellow"),
            (.systemGreen, "Green"), (.white, "White")]
        for (i, (color, name)) in colors.enumerated() {
            let btn = NSButton(frame: NSRect(x: x + CGFloat(i) * 30, y: 10, width: 24, height: 24))
            btn.wantsLayer = true
            btn.isBordered = false
            btn.layer?.backgroundColor = color.cgColor
            btn.layer?.cornerRadius = 12
            btn.tag = i
            btn.target = self
            btn.action = #selector(pickColor(_:))
            btn.toolTip = name
            bar.addSubview(btn)
        }
        x += CGFloat(colors.count) * 30 + 8

        // Separator
        let sep2 = NSBox(frame: NSRect(x: x, y: 6, width: 1, height: 28))
        sep2.boxType = .separator
        bar.addSubview(sep2)
        x += 8

        // Text mode toggle
        textModeButton = NSButton(frame: NSRect(x: x, y: 8, width: 28, height: 28))
        textModeButton.title = "T"
        textModeButton.font = .boldSystemFont(ofSize: 14)
        textModeButton.bezelStyle = .rounded
        textModeButton.target = self
        textModeButton.action = #selector(toggleTextMode)
        textModeButton.toolTip = "Text mode (T) — type at cursor, click to place"
        bar.addSubview(textModeButton)
        x += 34

        // Hint (right-aligned)
        let hint = NSTextField(labelWithString: "⌘⏎ Send  ·  Esc Cancel")
        hint.frame = NSRect(x: width - 170, y: 12, width: 160, height: 18)
        hint.alignment = .right
        hint.textColor = NSColor(white: 0.5, alpha: 1)
        hint.font = .systemFont(ofSize: 11)
        bar.addSubview(hint)

        return bar
    }

    func makeButton(_ title: String, action: Selector) -> NSButton {
        let btn = NSButton(title: title, target: self, action: action)
        btn.bezelStyle = .rounded
        btn.font = .systemFont(ofSize: 11)
        return btn
    }

    @objc func clearAll() {
        canvas.actions.removeAll()
        canvas.needsDisplay = true
    }

    @objc func undoLast() {
        canvas.undoLastAction()
    }

    @objc func pickColor(_ sender: NSButton) {
        let colors: [NSColor] = [.systemRed, .systemYellow, .systemGreen, .white]
        if sender.tag < colors.count {
            canvas.currentColor = colors[sender.tag]
        }
    }

    @objc func toggleTextMode() {
        isTextMode = !isTextMode
        canvas.isTextMode = isTextMode

        if isTextMode {
            textModeButton.state = .on
            textModeButton.wantsLayer = true
            textModeButton.layer?.backgroundColor = NSColor.systemBlue.withAlphaComponent(0.3).cgColor
            textModeButton.layer?.cornerRadius = 4
            canvas.pendingText = ""
            makeFirstResponder(canvas)
        } else {
            exitTextModeUI()
            canvas.commitPendingText()
            makeFirstResponder(canvas)
        }
        canvas.needsDisplay = true
    }

    func exitTextModeUI() {
        isTextMode = false
        canvas.isTextMode = false
        textModeButton.state = .off
        textModeButton.layer?.backgroundColor = nil
    }

    // MARK: - Keyboard

    override var canBecomeKey: Bool { true }
    override var canBecomeMain: Bool { true }

    // When window becomes key, ensure canvas is first responder
    override func becomeKey() {
        super.becomeKey()
        if firstResponder !== canvas {
            makeFirstResponder(canvas)
        }
    }

    override func keyDown(with event: NSEvent) {
        if !handleKey(event) {
            super.keyDown(with: event)
        }
    }

    @discardableResult
    func handleKey(_ event: NSEvent) -> Bool {
        let flags = event.modifierFlags.intersection(.deviceIndependentFlagsMask)
        let noModifiers = flags.isSubset(of: [.shift]) // shift is ok (for uppercase)

        // ⌘Enter → save and route
        if event.keyCode == 36 && flags.contains(.command) {
            saveAndRoute()
            return true
        }
        // ⌘Z → undo
        if event.keyCode == 6 && flags.contains(.command) {
            undoLast()
            return true
        }
        // Escape
        if event.keyCode == 53 {
            if isTextMode {
                toggleTextMode() // exit text mode, don't dismiss window
                return true
            }
            dismiss()
            return true
        }
        // 'T' key (keycode 17) with no cmd/ctrl → toggle text mode
        if event.keyCode == 17 && noModifiers && !isTextMode {
            toggleTextMode()
            return true
        }
        return false
    }

    // MARK: - Dismiss

    func dismiss() {
        guard !dismissed else { return }
        dismissed = true
        orderOut(nil)
        try? FileManager.default.removeItem(atPath: tempPath)
        onDismiss?()
    }

    override func close() { dismiss() }

    // MARK: - Save & Route

    func saveAndRoute() {
        guard !dismissed else { return }
        dismissed = true

        guard let composited = canvas.renderComposite() else { return }

        try? FileManager.default.createDirectory(
            at: SCREENSHOTS_DIR, withIntermediateDirectories: true)

        let filename = "sauron_annotated_\(Int(Date().timeIntervalSince1970 * 1000)).png"
        let savePath = SCREENSHOTS_DIR.appendingPathComponent(filename)

        guard let tiff = composited.tiffRepresentation,
              let rep = NSBitmapImageRep(data: tiff),
              let png = rep.representation(using: .png, properties: [:]) else {
            NSLog("SauronCapture: render failed")
            return
        }

        do {
            try png.write(to: savePath)
            NSLog("SauronCapture: saved %@", savePath.path)
        } catch {
            NSLog("SauronCapture: write failed: \(error)")
            return
        }

        let path = savePath.path

        DispatchQueue.global(qos: .utility).async { [self] in
            registerScreenshot(path)
        }

        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(path, forType: .string)

        orderOut(nil)

        DispatchQueue.main.asyncAfter(deadline: .now() + 0.2) { [self] in
            previousApp?.activate()
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.3) {
                self.simulatePaste()
                try? FileManager.default.removeItem(atPath: self.tempPath)
                self.onDismiss?()
            }
        }
    }

    func registerScreenshot(_ path: String) {
        let candidates = [
            FileManager.default.homeDirectoryForCurrentUser
                .appendingPathComponent("coding/sauron/sauron").path,
            "/usr/local/bin/sauron"
        ]
        guard let binary = candidates.first(where: {
            FileManager.default.isExecutableFile(atPath: $0)
        }) else { return }

        let sourceApp = previousApp?.localizedName ?? "unknown"
        let bundleID = previousApp?.bundleIdentifier ?? ""

        let task = Process()
        task.executableURL = URL(fileURLWithPath: binary)
        task.arguments = ["capture", "--register", path,
                          "--source-app", sourceApp, "--bundle-id", bundleID]
        task.standardOutput = FileHandle.nullDevice
        task.standardError = FileHandle.nullDevice
        try? task.run()
        task.waitUntilExit()
    }

    func simulatePaste() {
        let src = CGEventSource(stateID: .hidSystemState)
        if let down = CGEvent(keyboardEventSource: src, virtualKey: 0x09, keyDown: true) {
            down.flags = .maskCommand
            down.post(tap: .cghidEventTap)
        }
        if let up = CGEvent(keyboardEventSource: src, virtualKey: 0x09, keyDown: false) {
            up.flags = .maskCommand
            up.post(tap: .cghidEventTap)
        }
    }
}

// ─── Annotation View (Drawing + Live Text Canvas) ───────────────────────────

enum AnnotationAction {
    case stroke(NSBezierPath, NSColor)
    case text(TextAnnotation)
}

class AnnotationView: NSView {
    var image: NSImage?
    var actions: [AnnotationAction] = []  // Unified undo stack
    var currentPath: NSBezierPath?
    var currentColor: NSColor = .systemRed
    var isTextMode = false

    // Convenience accessors
    var strokes: [(NSBezierPath, NSColor)] {
        actions.compactMap { if case .stroke(let p, let c) = $0 { return (p, c) } else { return nil } }
    }
    var textAnnotations: [TextAnnotation] {
        actions.compactMap { if case .text(let t) = $0 { return t } else { return nil } }
    }

    // Live text state — text follows cursor, typed character by character
    var pendingText = ""
    var mousePos = NSPoint(x: 100, y: 100)

    override var acceptsFirstResponder: Bool { true }

    override func draw(_ dirtyRect: NSRect) {
        super.draw(dirtyRect)
        image?.draw(in: bounds)

        // Committed strokes
        for (path, color) in strokes {
            color.setStroke()
            path.lineWidth = LINE_WIDTH
            path.lineCapStyle = .round
            path.lineJoinStyle = .round
            path.stroke()
        }

        // In-progress stroke
        if let active = currentPath {
            currentColor.setStroke()
            active.lineWidth = LINE_WIDTH
            active.lineCapStyle = .round
            active.lineJoinStyle = .round
            active.stroke()
        }

        // Committed text annotations
        for ann in textAnnotations {
            drawText(ann.text, at: ann.point, color: ann.color, fontSize: ann.fontSize)
        }

        // Live pending text at cursor (ghost preview)
        if isTextMode && !pendingText.isEmpty {
            drawText(pendingText, at: mousePos, color: currentColor.withAlphaComponent(0.85),
                     fontSize: TEXT_FONT_SIZE)
        }

        // Text mode cursor indicator
        if isTextMode {
            let cursorColor = currentColor.withAlphaComponent(0.5)
            cursorColor.setStroke()
            let cursor = NSBezierPath()
            cursor.move(to: NSPoint(x: mousePos.x, y: mousePos.y - 2))
            cursor.line(to: NSPoint(x: mousePos.x, y: mousePos.y + TEXT_FONT_SIZE + 2))
            cursor.lineWidth = 1.5
            cursor.stroke()
        }
    }

    func drawText(_ text: String, at point: NSPoint, color: NSColor, fontSize: CGFloat) {
        let font = NSFont.boldSystemFont(ofSize: fontSize)
        let attrs: [NSAttributedString.Key: Any] = [
            .font: font,
            .foregroundColor: color
        ]
        let attrStr = NSAttributedString(string: text, attributes: attrs)
        let size = attrStr.size()

        // Dark background pill for contrast
        let padding: CGFloat = 4
        let bgRect = NSRect(
            x: point.x - padding,
            y: point.y - padding,
            width: size.width + padding * 2,
            height: size.height + padding * 2)
        let bg = NSBezierPath(roundedRect: bgRect, xRadius: 4, yRadius: 4)
        NSColor.black.withAlphaComponent(0.65).setFill()
        bg.fill()

        attrStr.draw(at: point)
    }

    // MARK: - Mouse

    override func mouseMoved(with event: NSEvent) {
        mousePos = convert(event.locationInWindow, from: nil)
        if isTextMode { needsDisplay = true }
    }

    override func mouseDown(with event: NSEvent) {
        let pt = convert(event.locationInWindow, from: nil)

        if isTextMode {
            // Click commits the pending text and exits text mode
            if !pendingText.isEmpty {
                actions.append(.text(TextAnnotation(
                    text: pendingText, point: pt, color: currentColor,
                    fontSize: TEXT_FONT_SIZE)))
                pendingText = ""
            }
            isTextMode = false
            if let win = window as? AnnotationWindow { win.exitTextModeUI() }
            needsDisplay = true
            return
        }

        window?.makeFirstResponder(self)
        currentPath = NSBezierPath()
        currentPath?.move(to: pt)
    }

    override func mouseDragged(with event: NSEvent) {
        guard !isTextMode else { return }
        let pt = convert(event.locationInWindow, from: nil)
        currentPath?.line(to: pt)
        needsDisplay = true
    }

    override func mouseUp(with event: NSEvent) {
        guard !isTextMode else { return }
        if let path = currentPath {
            actions.append(.stroke(path, currentColor))
        }
        currentPath = nil
        needsDisplay = true
    }

    // MARK: - Keyboard

    override func keyDown(with event: NSEvent) {
        // In text mode, capture typing
        if isTextMode {
            let flags = event.modifierFlags.intersection(.deviceIndependentFlagsMask)

            // Let ⌘S, ⌘Z, Escape pass through to window handler
            if flags.contains(.command) || event.keyCode == 53 {
                if let win = window as? AnnotationWindow, win.handleKey(event) { return }
                super.keyDown(with: event)
                return
            }

            // Backspace
            if event.keyCode == 51 {
                if !pendingText.isEmpty {
                    pendingText.removeLast()
                    needsDisplay = true
                }
                return
            }

            // Return commits at current mouse pos and exits text mode
            if event.keyCode == 36 {
                if !pendingText.isEmpty {
                    actions.append(.text(TextAnnotation(
                        text: pendingText, point: mousePos, color: currentColor,
                        fontSize: TEXT_FONT_SIZE)))
                    pendingText = ""
                }
                isTextMode = false
                if let win = window as? AnnotationWindow { win.exitTextModeUI() }
                needsDisplay = true
                return
            }

            // Regular characters
            if let chars = event.characters, !chars.isEmpty {
                pendingText += chars
                needsDisplay = true
            }
            return
        }

        // Not in text mode — delegate to window
        if let win = window as? AnnotationWindow, win.handleKey(event) { return }
        super.keyDown(with: event)
    }

    // MARK: - Commit / Render

    func commitPendingText() {
        if !pendingText.isEmpty {
            actions.append(.text(TextAnnotation(
                text: pendingText, point: mousePos, color: currentColor,
                fontSize: TEXT_FONT_SIZE)))
            pendingText = ""
        }
    }

    func undoLastAction() {
        if !actions.isEmpty {
            actions.removeLast()
            needsDisplay = true
        }
    }

    func renderComposite() -> NSImage? {
        guard let image = image else { return nil }
        let size = bounds.size
        let result = NSImage(size: size)

        result.lockFocus()
        image.draw(in: NSRect(origin: .zero, size: size))

        for (path, color) in strokes {
            color.setStroke()
            path.lineWidth = LINE_WIDTH
            path.lineCapStyle = .round
            path.lineJoinStyle = .round
            path.stroke()
        }

        for ann in textAnnotations {
            drawText(ann.text, at: ann.point, color: ann.color, fontSize: ann.fontSize)
        }

        result.unlockFocus()
        return result
    }
}

// ─── Gallery / Library Window ────────────────────────────────────────────────

class GalleryWindow: NSWindow {
    var scrollView: NSScrollView!
    var gridView: NSView!
    var images: [(path: String, date: Date)] = []
    var previewWindows: [NSWindow] = []  // Keep strong refs

    init() {
        let screen = NSScreen.main!.visibleFrame
        let w: CGFloat = min(1200, screen.width * 0.85)
        let h: CGFloat = min(850, screen.height * 0.85)
        let frame = NSRect(
            x: screen.midX - w / 2,
            y: screen.midY - h / 2,
            width: w, height: h)

        super.init(
            contentRect: frame,
            styleMask: [.titled, .closable, .miniaturizable, .resizable],
            backing: .buffered, defer: false)

        title = "Sauron Library"
        isReleasedWhenClosed = false
        minSize = NSSize(width: 500, height: 400)
        backgroundColor = NSColor(white: 0.08, alpha: 1)

        setupUI()
        loadImages()
    }

    func setupUI() {
        let container = NSView(frame: contentView!.bounds)
        container.autoresizingMask = [.width, .height]
        container.wantsLayer = true
        container.layer?.backgroundColor = NSColor(white: 0.08, alpha: 1).cgColor

        // Header
        let header = NSView(frame: NSRect(
            x: 0, y: container.bounds.height - 50,
            width: container.bounds.width, height: 50))
        header.autoresizingMask = [.width, .minYMargin]
        header.wantsLayer = true
        header.layer?.backgroundColor = NSColor(white: 0.1, alpha: 1).cgColor

        let titleLabel = NSTextField(labelWithString: "Screenshots")
        titleLabel.font = .boldSystemFont(ofSize: 18)
        titleLabel.textColor = .white
        titleLabel.frame = NSRect(x: 16, y: 12, width: 200, height: 24)
        header.addSubview(titleLabel)

        let refreshBtn = NSButton(title: "Refresh", target: self, action: #selector(refreshGallery))
        refreshBtn.bezelStyle = .rounded
        refreshBtn.frame = NSRect(x: container.bounds.width - 90, y: 10, width: 78, height: 28)
        refreshBtn.autoresizingMask = [.minXMargin]
        header.addSubview(refreshBtn)

        container.addSubview(header)

        // Scroll view for grid
        scrollView = NSScrollView(frame: NSRect(
            x: 0, y: 0,
            width: container.bounds.width,
            height: container.bounds.height - 50))
        scrollView.autoresizingMask = [.width, .height]
        scrollView.hasVerticalScroller = true
        scrollView.drawsBackground = false
        scrollView.backgroundColor = .clear

        gridView = NSView()
        gridView.wantsLayer = true
        scrollView.documentView = gridView

        container.addSubview(scrollView)
        contentView = container
    }

    func loadImages() {
        images.removeAll()

        let fm = FileManager.default
        let dir = SCREENSHOTS_DIR.path

        guard let entries = try? fm.contentsOfDirectory(atPath: dir) else { return }

        for name in entries {
            let ext = (name as NSString).pathExtension.lowercased()
            guard ext == "png" || ext == "jpg" || ext == "jpeg" else { continue }

            let fullPath = (dir as NSString).appendingPathComponent(name)
            if let attrs = try? fm.attributesOfItem(atPath: fullPath),
               let date = attrs[.modificationDate] as? Date {
                images.append((path: fullPath, date: date))
            }
        }

        // Most recent first
        images.sort { $0.date > $1.date }
        layoutGrid()
    }

    func layoutGrid() {
        gridView.subviews.forEach { $0.removeFromSuperview() }

        let scrollWidth = scrollView.bounds.width
        let scrollHeight = scrollView.bounds.height
        let padding: CGFloat = 16
        let thumbSize = GALLERY_THUMB_SIZE
        let cols = max(1, Int((scrollWidth - padding) / (thumbSize + padding)))
        let rows = (images.count + cols - 1) / cols
        let cardHeight = thumbSize + 28
        let contentHeight = CGFloat(rows) * (cardHeight + padding) + padding
        let docHeight = max(contentHeight, scrollHeight)

        gridView.frame = NSRect(x: 0, y: 0, width: scrollWidth, height: docHeight)

        for (i, item) in images.enumerated() {
            let col = i % cols
            let row = i / cols

            let x = padding + CGFloat(col) * (thumbSize + padding)
            // Pin to top of document view
            let y = docHeight - padding - CGFloat(row + 1) * (cardHeight + padding)

            let card = makeThumbnailCard(
                path: item.path, date: item.date,
                frame: NSRect(x: x, y: y, width: thumbSize, height: cardHeight))
            gridView.addSubview(card)
        }

        if images.isEmpty {
            let empty = NSTextField(labelWithString: "No screenshots yet.\nCapture one with ⌃⇧S or the ◎ menu.")
            empty.font = .systemFont(ofSize: 14)
            empty.textColor = NSColor(white: 0.4, alpha: 1)
            empty.alignment = .center
            empty.frame = NSRect(x: 0, y: docHeight / 2 - 20,
                                 width: scrollWidth, height: 40)
            gridView.addSubview(empty)
        }

        // Scroll to top
        if let docView = scrollView.documentView {
            docView.scroll(NSPoint(x: 0, y: docHeight))
        }
    }

    func makeThumbnailCard(path: String, date: Date, frame: NSRect) -> NSView {
        let card = NSView(frame: frame)
        card.wantsLayer = true
        card.layer?.backgroundColor = NSColor(white: 0.14, alpha: 1).cgColor
        card.layer?.cornerRadius = 8

        // Thumbnail image
        let imgView = ClickableImageView(frame: NSRect(
            x: 4, y: 28, width: frame.width - 8, height: frame.height - 32))
        imgView.imageScaling = .scaleProportionallyUpOrDown
        imgView.wantsLayer = true
        imgView.layer?.cornerRadius = 6
        imgView.layer?.masksToBounds = true
        imgView.imagePath = path
        imgView.onClick = { p in
            copyImageToClipboard(path: p)
            card.layer?.backgroundColor = NSColor.systemBlue.withAlphaComponent(0.3).cgColor
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.3) {
                card.layer?.backgroundColor = NSColor(white: 0.14, alpha: 1).cgColor
            }
        }
        imgView.onDoubleClick = { [weak self] p in self?.openFullPreview(path: p) }

        // Load thumbnail async (lockFocus must happen on main thread)
        DispatchQueue.global(qos: .utility).async {
            guard let img = NSImage(contentsOfFile: path) else { return }
            DispatchQueue.main.async {
                let thumbSize = NSSize(width: GALLERY_THUMB_SIZE * 2, height: GALLERY_THUMB_SIZE * 2)
                let thumb = NSImage(size: thumbSize)
                thumb.lockFocus()
                img.draw(in: NSRect(origin: .zero, size: thumbSize),
                         from: NSRect(origin: .zero, size: img.size),
                         operation: .copy, fraction: 1.0)
                thumb.unlockFocus()
                imgView.image = thumb
            }
        }
        card.addSubview(imgView)

        // Date label
        let fmt = DateFormatter()
        fmt.dateFormat = "MMM d, h:mm a"
        let label = NSTextField(labelWithString: fmt.string(from: date))
        label.font = .systemFont(ofSize: 10)
        label.textColor = NSColor(white: 0.5, alpha: 1)
        label.frame = NSRect(x: 6, y: 6, width: frame.width - 12, height: 16)
        card.addSubview(label)

        return card
    }

    func openFullPreview(path: String) {
        guard let image = NSImage(contentsOfFile: path) else { return }

        let screen = NSScreen.main!.visibleFrame
        let maxW = screen.width * 0.9
        let maxH = screen.height * 0.9
        let scale = min(maxW / image.size.width, maxH / image.size.height, 1.0)
        let w = max(image.size.width * scale, 600)
        let h = max(image.size.height * scale, 400)

        let previewWin = NSWindow(
            contentRect: NSRect(x: screen.midX - w / 2, y: screen.midY - h / 2, width: w, height: h),
            styleMask: [.titled, .closable, .resizable],
            backing: .buffered, defer: false)
        previewWin.title = "\((path as NSString).lastPathComponent) — click to copy"
        previewWin.isReleasedWhenClosed = false
        previewWin.backgroundColor = NSColor(white: 0.05, alpha: 1)

        let clickView = ClickableImageView(frame: NSRect(x: 0, y: 0, width: w, height: h))
        clickView.image = image
        clickView.imageScaling = .scaleProportionallyUpOrDown
        clickView.autoresizingMask = [.width, .height]
        clickView.imagePath = path
        clickView.onClick = { p in
            copyImageToClipboard(path: p)
            previewWin.title = "Copied! — \((p as NSString).lastPathComponent)"
            DispatchQueue.main.asyncAfter(deadline: .now() + 1.5) {
                previewWin.title = "\((p as NSString).lastPathComponent) — click to copy"
            }
        }
        previewWin.contentView = clickView

        previewWin.makeKeyAndOrderFront(nil)
        previewWindows.append(previewWin)
    }

    @objc func refreshGallery() {
        loadImages()
    }

    // Re-layout on resize
    override func setFrame(_ frameRect: NSRect, display flag: Bool) {
        super.setFrame(frameRect, display: flag)
        if !images.isEmpty { layoutGrid() }
    }
}

// ─── Clipboard Helper ────────────────────────────────────────────────────────

func copyImageToClipboard(path: String) {
    guard let image = NSImage(contentsOfFile: path) else { return }
    let pb = NSPasteboard.general
    pb.clearContents()
    // Copy both the image and the file URL so it works in any context
    pb.writeObjects([image])
    pb.setString(path, forType: .string)
    // Also add as file URL for drag-drop / Finder paste
    if let url = URL(string: "file://" + path) {
        pb.setData(url.absoluteString.data(using: .utf8), forType: .fileURL)
    }
}

// ─── Clickable Image View ────────────────────────────────────────────────────

class ClickableImageView: NSImageView {
    var imagePath: String = ""
    var onClick: ((String) -> Void)?
    var onDoubleClick: ((String) -> Void)?

    override func mouseDown(with event: NSEvent) {
        if event.clickCount == 2 {
            onDoubleClick?(imagePath)
        } else if event.clickCount == 1 {
            onClick?(imagePath)
        }
    }
}
