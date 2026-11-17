package com.cmacle.thehousealwayswins;

import android.content.Context;
import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.Paint;
import android.graphics.Rect;
import android.os.Build;
import android.view.DisplayCutout;
import android.view.View;
import android.view.WindowInsets;
import android.text.InputType;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.SurfaceHolder;
import android.view.SurfaceView;
import android.view.ViewTreeObserver;
import android.view.inputmethod.BaseInputConnection;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputConnection;
import android.view.inputmethod.InputMethodManager;

import java.nio.ByteBuffer;

import mobile.Mobile;

/**
 * Draws the game's pixel-art canvas to fill the screen in either orientation.
 * The canvas size follows the screen's shape (the Go side picks it), is
 * scaled up by whole pixels and then smoothly to the exact screen size, so it
 * stays sharp. When the keyboard is up, the screen pans so the text box you're
 * typing in sits just above it.
 */
public class GameView extends SurfaceView implements SurfaceHolder.Callback, Runnable {
    private Bitmap bitmap;       // the canvas, at its own size
    private Bitmap big;          // the canvas scaled up by whole pixels
    private Canvas bigCanvas;
    private int bw, bh, k = 1;
    private final Paint nearest = new Paint();
    private final Paint smooth = new Paint();
    private final Rect dst = new Rect();
    private final Rect bigSrc = new Rect();
    private final Rect visible = new Rect();

    private Thread thread;
    private volatile boolean running;
    private volatile boolean surfaceReady;
    private volatile int surfW, surfH;
    private volatile int ox, oy, dw = 1, dh = 1, pan;
    private volatile int imeBottom;   // keyboard height from window insets
    private volatile int safeTop, safeBottom, safeLeft, safeRight;
    private volatile boolean wantKeyboard;

    public GameView(Context context) {
        super(context);
        getHolder().addCallback(this);
        setFocusable(true);
        setFocusableInTouchMode(true);
        nearest.setFilterBitmap(false);
        nearest.setAntiAlias(false);
        nearest.setDither(false);
        smooth.setFilterBitmap(true);
        getViewTreeObserver().addOnGlobalLayoutListener(new ViewTreeObserver.OnGlobalLayoutListener() {
            @Override
            public void onGlobalLayout() {
                layoutGame();
            }
        });
        setOnApplyWindowInsetsListener(new View.OnApplyWindowInsetsListener() {
            @Override
            @SuppressWarnings("deprecation")
            public WindowInsets onApplyWindowInsets(View v, WindowInsets insets) {
                int ime = 0;
                if (Build.VERSION.SDK_INT >= 30) {
                    ime = insets.getInsets(WindowInsets.Type.ime()).bottom;
                } else {
                    ime = insets.getSystemWindowInsetBottom();
                }
                imeBottom = ime;
                if (Build.VERSION.SDK_INT >= 28 && insets.getDisplayCutout() != null) {
                    DisplayCutout c = insets.getDisplayCutout();
                    safeTop = c.getSafeInsetTop();
                    safeBottom = c.getSafeInsetBottom();
                    safeLeft = c.getSafeInsetLeft();
                    safeRight = c.getSafeInsetRight();
                } else {
                    safeTop = safeBottom = safeLeft = safeRight = 0;
                }
                layoutGame();
                return insets;
            }
        });
    }

    /** Height of the on-screen keyboard covering the bottom of the view. */
    private int keyboardHeight() {
        int h = surfH;
        int kb = imeBottom;
        getWindowVisibleDisplayFrame(visible);
        int[] loc = new int[2];
        getLocationOnScreen(loc);
        int visBottom = visible.bottom - loc[1];
        if (visBottom > 0 && visBottom < h - h / 6) {
            kb = Math.max(kb, h - visBottom);
        }
        return kb > h / 6 ? kb : 0;
    }

    private void layoutGame() {
        int w = surfW, h = surfH;
        if (w <= 0 || h <= 0) return;
        int aw = w - safeLeft - safeRight, ah = h - safeTop - safeBottom;
        Mobile.layout(aw, ah);
        int W = (int) Mobile.width(), H = (int) Mobile.height();
        float s = Math.min(aw / (float) W, ah / (float) H);
        int nw = Math.max(1, Math.min(aw, Math.round(W * s)));
        int nh = Math.max(1, Math.min(ah, Math.round(H * s)));
        int nk = Math.max(1, (int) s);
        int nox = safeLeft + (aw - nw) / 2;
        int noy = safeTop + (ah - nh) / 2;
        // pan up so the text box being typed in stays above the keyboard
        int p = 0;
        int kb = keyboardHeight();
        double fb = Mobile.focusBottom();
        if (kb > 0 && fb >= 0) {
            int fieldBottom = noy + (int) Math.round((fb + 8) * nh / (double) H);
            int limit = h - kb;
            if (fieldBottom > limit) p = Math.min(fieldBottom - limit, nh);
        }
        synchronized (dst) {
            dw = nw;
            dh = nh;
            k = nk;
            ox = nox;
            oy = noy;
            pan = p;
            dst.set(ox, oy - pan, ox + dw, oy - pan + dh);
        }
    }

    // ------------------------------------------------------------ surface

    @Override
    public void surfaceCreated(SurfaceHolder holder) {
        surfaceReady = true;
        resume();
    }

    @Override
    public void surfaceChanged(SurfaceHolder holder, int format, int width, int height) {
        surfW = width;
        surfH = height;
        layoutGame();
    }

    @Override
    public void surfaceDestroyed(SurfaceHolder holder) {
        surfaceReady = false;
        pause();
    }

    public void resume() {
        if (running || !surfaceReady) return;
        running = true;
        thread = new Thread(this, "game");
        thread.start();
    }

    public void pause() {
        running = false;
        Thread t = thread;
        thread = null;
        if (t != null) {
            try {
                t.join(500);
            } catch (InterruptedException ignored) {
            }
        }
    }

    @Override
    public void run() {
        final long frameNs = 1_000_000_000L / 60;
        long next = System.nanoTime();
        int frames = 0;
        while (running) {
            byte[] px = Mobile.frame();
            int W = (int) Mobile.width(), H = (int) Mobile.height();
            if (bitmap == null || bitmap.getWidth() != W || bitmap.getHeight() != H) {
                bitmap = Bitmap.createBitmap(W, H, Bitmap.Config.ARGB_8888);
            }
            if (px != null && px.length == W * H * 4) {
                bitmap.copyPixelsFromBuffer(ByteBuffer.wrap(px));
            }
            if (++frames % 10 == 0) post(this::layoutGame); // keyboard / focus moved
            SurfaceHolder holder = getHolder();
            Canvas c = null;
            try {
                c = holder.lockCanvas();
                if (c != null) {
                    c.drawColor(Color.BLACK);
                    synchronized (dst) {
                        int kw = W * k, kh = H * k;
                        if (kw == dst.width() && kh == dst.height()) {
                            c.drawBitmap(bitmap, null, dst, nearest);
                        } else {
                            if (big == null || bw != kw || bh != kh) {
                                big = Bitmap.createBitmap(kw, kh, Bitmap.Config.ARGB_8888);
                                bigCanvas = new Canvas(big);
                                bw = kw;
                                bh = kh;
                            }
                            bigSrc.set(0, 0, kw, kh);
                            bigCanvas.drawBitmap(bitmap, null, bigSrc, nearest);
                            c.drawBitmap(big, null, dst, smooth);
                        }
                    }
                }
            } catch (Exception ignored) {
            } finally {
                if (c != null) {
                    try {
                        holder.unlockCanvasAndPost(c);
                    } catch (Exception ignored) {
                    }
                }
            }
            next += frameNs;
            long sleep = next - System.nanoTime();
            if (sleep > 0) {
                try {
                    Thread.sleep(sleep / 1_000_000L, (int) (sleep % 1_000_000L));
                } catch (InterruptedException ignored) {
                }
            } else if (sleep < -100_000_000L) {
                next = System.nanoTime();
            }
        }
    }

    // ------------------------------------------------------------ touch

    @Override
    public boolean onTouchEvent(MotionEvent e) {
        int action = e.getActionMasked();
        int W = (int) Mobile.width(), H = (int) Mobile.height();
        double x = (e.getX() - ox) * W / (double) dw;
        double y = (e.getY() - (oy - pan)) * H / (double) dh;
        switch (action) {
            case MotionEvent.ACTION_DOWN:
                Mobile.touch(Mobile.TouchDown, x, y);
                break;
            case MotionEvent.ACTION_MOVE:
                Mobile.touch(Mobile.TouchMove, x, y);
                break;
            case MotionEvent.ACTION_UP:
            case MotionEvent.ACTION_CANCEL:
                Mobile.touch(Mobile.TouchUp, x, y);
                performClick();
                break;
            default:
                break;
        }
        return true;
    }

    @Override
    public boolean performClick() {
        return super.performClick();
    }

    // ------------------------------------------------------------ keyboard

    public void showKeyboard(boolean show) {
        final InputMethodManager imm = (InputMethodManager) getContext().getSystemService(Context.INPUT_METHOD_SERVICE);
        if (imm == null) return;
        wantKeyboard = show;
        if (show) {
            if (!hasFocus()) requestFocus();
            imm.restartInput(this);
            if (Build.VERSION.SDK_INT >= 30 && getWindowInsetsController() != null) {
                getWindowInsetsController().show(WindowInsets.Type.ime());
            }
            imm.showSoftInput(this, 0);
            // some keyboards ignore the first request: try again, harder
            postDelayed(new Runnable() {
                @Override
                @SuppressWarnings("deprecation")
                public void run() {
                    if (wantKeyboard && keyboardHeight() == 0) {
                        requestFocus();
                        imm.showSoftInput(GameView.this, InputMethodManager.SHOW_FORCED);
                    }
                }
            }, 300);
        } else {
            imm.hideSoftInputFromWindow(getWindowToken(), 0);
        }
        layoutGame();
    }

    @Override
    public boolean onCheckIsTextEditor() {
        return true;
    }

    @Override
    public InputConnection onCreateInputConnection(EditorInfo out) {
        // A plain "visible password" field: no autocorrect or word suggestions,
        // so keyboards send characters one at a time.
        out.inputType = InputType.TYPE_CLASS_TEXT
                | InputType.TYPE_TEXT_VARIATION_VISIBLE_PASSWORD
                | InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS;
        out.imeOptions = EditorInfo.IME_FLAG_NO_EXTRACT_UI
                | EditorInfo.IME_FLAG_NO_FULLSCREEN
                | EditorInfo.IME_ACTION_DONE;
        return new BaseInputConnection(this, false) {
            @Override
            public boolean commitText(CharSequence text, int newCursorPosition) {
                Mobile.text(text.toString());
                return true;
            }

            @Override
            public boolean setComposingText(CharSequence text, int newCursorPosition) {
                return true; // nothing is composed in a visible-password field
            }

            @Override
            public boolean finishComposingText() {
                return true;
            }

            @Override
            public boolean deleteSurroundingText(int beforeLength, int afterLength) {
                for (int i = 0; i < beforeLength; i++) {
                    Mobile.key(Mobile.KeyBackspace);
                }
                for (int i = 0; i < afterLength; i++) {
                    Mobile.key(Mobile.KeyDelete);
                }
                return true;
            }

            @Override
            public boolean performEditorAction(int actionCode) {
                Mobile.key(Mobile.KeyEnter);
                return true;
            }
        };
    }

    @Override
    public boolean onKeyDown(int keyCode, KeyEvent event) {
        switch (keyCode) {
            case KeyEvent.KEYCODE_DEL:
                Mobile.key(Mobile.KeyBackspace);
                return true;
            case KeyEvent.KEYCODE_FORWARD_DEL:
                Mobile.key(Mobile.KeyDelete);
                return true;
            case KeyEvent.KEYCODE_ENTER:
            case KeyEvent.KEYCODE_NUMPAD_ENTER:
                Mobile.key(Mobile.KeyEnter);
                return true;
            case KeyEvent.KEYCODE_TAB:
                Mobile.key(Mobile.KeyTab);
                return true;
            case KeyEvent.KEYCODE_DPAD_LEFT:
                Mobile.key(Mobile.KeyLeft);
                return true;
            case KeyEvent.KEYCODE_DPAD_RIGHT:
                Mobile.key(Mobile.KeyRight);
                return true;
            case KeyEvent.KEYCODE_ESCAPE:
                Mobile.key(Mobile.KeyBack);
                return true;
            case KeyEvent.KEYCODE_BACK:
                return super.onKeyDown(keyCode, event);
            default:
                int ch = event.getUnicodeChar();
                if (ch > 0) {
                    Mobile.keyChar(ch);
                    return true;
                }
                return super.onKeyDown(keyCode, event);
        }
    }
}
