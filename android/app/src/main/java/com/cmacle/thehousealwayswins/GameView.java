package com.cmacle.thehousealwayswins;

import android.content.Context;
import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.Paint;
import android.graphics.Rect;
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
 * Draws the game's 640x360 frame scaled up by whole pixels (pixel art stays
 * sharp), and turns touches and keys into game input. When the on-screen
 * keyboard is up, the game is fitted into the space above it.
 */
public class GameView extends SurfaceView implements SurfaceHolder.Callback, Runnable {
    private final int W = (int) Mobile.width();
    private final int H = (int) Mobile.height();
    private final Bitmap bitmap = Bitmap.createBitmap(W, H, Bitmap.Config.ARGB_8888);
    private final Paint paint = new Paint();
    private final Rect dst = new Rect();
    private final Rect visible = new Rect();

    private Thread thread;
    private volatile boolean running;
    private volatile boolean surfaceReady;
    private int surfW, surfH;
    private volatile int ox, oy, dw = 1, dh = 1;

    public GameView(Context context) {
        super(context);
        getHolder().addCallback(this);
        setFocusable(true);
        setFocusableInTouchMode(true);
        paint.setFilterBitmap(false);
        paint.setAntiAlias(false);
        paint.setDither(false);
        // refit the game above the keyboard when it opens or closes
        getViewTreeObserver().addOnGlobalLayoutListener(new ViewTreeObserver.OnGlobalLayoutListener() {
            @Override
            public void onGlobalLayout() {
                layoutGame();
            }
        });
    }

    private void layoutGame() {
        int w = surfW, h = surfH;
        if (w <= 0 || h <= 0) return;
        getWindowVisibleDisplayFrame(visible);
        int[] loc = new int[2];
        getLocationOnScreen(loc);
        int availH = h;
        int visBottom = visible.bottom - loc[1];
        if (visBottom > 0 && visBottom < h - h / 6) {
            availH = visBottom; // keyboard is up
        }
        int s = Math.min(w / W, availH / H);
        int nw, nh;
        if (s >= 1) {
            nw = W * s;
            nh = H * s;
        } else {
            nw = w;
            nh = w * H / W;
            if (nh > availH) {
                nh = availH;
                nw = availH * W / H;
            }
        }
        dw = Math.max(nw, 1);
        dh = Math.max(nh, 1);
        ox = (w - dw) / 2;
        oy = availH < h ? Math.max(0, (availH - dh) / 2) : (h - dh) / 2;
        synchronized (dst) {
            dst.set(ox, oy, ox + dw, oy + dh);
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
        while (running) {
            byte[] px = Mobile.frame();
            if (px != null && px.length == W * H * 4) {
                bitmap.copyPixelsFromBuffer(ByteBuffer.wrap(px));
            }
            SurfaceHolder holder = getHolder();
            Canvas c = null;
            try {
                c = holder.lockCanvas();
                if (c != null) {
                    c.drawColor(Color.BLACK);
                    synchronized (dst) {
                        c.drawBitmap(bitmap, null, dst, paint);
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
        double x = (e.getX() - ox) * W / (double) dw;
        double y = (e.getY() - oy) * H / (double) dh;
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
        InputMethodManager imm = (InputMethodManager) getContext().getSystemService(Context.INPUT_METHOD_SERVICE);
        if (imm == null) return;
        if (show) {
            requestFocus();
            imm.restartInput(this);
            imm.showSoftInput(this, InputMethodManager.SHOW_IMPLICIT);
        } else {
            imm.hideSoftInputFromWindow(getWindowToken(), 0);
        }
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
