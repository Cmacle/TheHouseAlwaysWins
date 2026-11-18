package com.cmacle.thehousealwayswins;

import android.app.Activity;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.content.Intent;
import android.media.AudioAttributes;
import android.media.SoundPool;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.provider.Settings;
import android.util.Log;
import android.view.View;
import android.view.WindowManager;
import android.widget.Toast;

import java.io.File;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.util.HashMap;
import java.util.Map;

import mobile.Host;
import mobile.Mobile;

/**
 * The whole game runs in Go (see /mobile); this activity hosts the drawing
 * surface and provides sound, the picture picker, the clipboard and the
 * on-screen keyboard.
 */
public class MainActivity extends Activity implements Host {
    private static final String TAG = "HouseAlwaysWins";
    private static final int PICK_IMAGE = 42;

    private GameView view;
    private SoundPool pool;
    private final Map<String, Integer> sounds = new HashMap<>();

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);

        File ext = getExternalFilesDir(null);
        File assets = new File(ext != null ? ext : getFilesDir(), "assets");
        try {
            Mobile.start(getFilesDir().getAbsolutePath(), assets.getAbsolutePath(), this);
        } catch (Exception e) {
            Log.e(TAG, "start failed", e);
            Toast.makeText(this, "Could not start: " + e.getMessage(), Toast.LENGTH_LONG).show();
        }
        loadSounds();

        view = new GameView(this);
        setContentView(view);
        if (Build.VERSION.SDK_INT >= 30) {
            // so the view gets keyboard insets even in fullscreen
            getWindow().setDecorFitsSystemWindows(false);
        }
        hideSystemUi();
    }

    private void loadSounds() {
        AudioAttributes attrs = new AudioAttributes.Builder()
                .setUsage(AudioAttributes.USAGE_GAME)
                .setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION)
                .build();
        pool = new SoundPool.Builder().setMaxStreams(6).setAudioAttributes(attrs).build();
        for (String name : Mobile.soundNames().split(",")) {
            byte[] data = Mobile.soundData(name);
            if (data == null || data.length == 0) continue;
            try {
                File f = new File(getCacheDir(), name + ".wav");
                try (OutputStream out = new FileOutputStream(f)) {
                    out.write(data);
                }
                sounds.put(name, pool.load(f.getAbsolutePath(), 1));
            } catch (Exception e) {
                Log.w(TAG, "sound " + name, e);
            }
        }
    }

    @SuppressWarnings("deprecation")
    private void hideSystemUi() {
        View decor = getWindow().getDecorView();
        decor.setSystemUiVisibility(
                View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY
                        | View.SYSTEM_UI_FLAG_LAYOUT_STABLE
                        | View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
                        | View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
                        | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
                        | View.SYSTEM_UI_FLAG_FULLSCREEN);
        if (Build.VERSION.SDK_INT >= 28) {
            getWindow().getAttributes().layoutInDisplayCutoutMode =
                    WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES;
        }
    }

    @Override
    public void onWindowFocusChanged(boolean hasFocus) {
        super.onWindowFocusChanged(hasFocus);
        if (hasFocus) hideSystemUi();
    }

    @Override
    protected void onResume() {
        super.onResume();
        hideSystemUi();
        if (pendingUpdate != null && (Build.VERSION.SDK_INT < 26 || getPackageManager().canRequestPackageInstalls())) {
            String p = pendingUpdate;
            pendingUpdate = null;
            installUpdate(p);
        }
        if (view != null) view.resume();
    }

    @Override
    protected void onPause() {
        super.onPause();
        if (view != null) view.pause();
        Mobile.pause();
    }

    @Override
    @SuppressWarnings("deprecation")
    public void onBackPressed() {
        // Back closes windows in the game (like Escape) instead of leaving.
        Mobile.key(Mobile.KeyBack);
    }

    // ------------------------------------------------------------------ Host

    @Override
    public void playSound(String name, double volume) {
        Integer id = sounds.get(name);
        if (id != null && pool != null) {
            float v = (float) Math.max(0, Math.min(1, volume));
            pool.play(id, v, v, 1, 0, 1f);
        }
    }

    @Override
    public void pickImage() {
        runOnUiThread(() -> {
            Intent i = new Intent(Intent.ACTION_GET_CONTENT);
            i.setType("image/*");
            i.addCategory(Intent.CATEGORY_OPENABLE);
            try {
                startActivityForResult(Intent.createChooser(i, "Choose a profile picture"), PICK_IMAGE);
            } catch (Exception e) {
                Toast.makeText(this, "No picture picker available", Toast.LENGTH_SHORT).show();
            }
        });
    }

    @Override
    public String clipboardText() {
        try {
            ClipboardManager cm = (ClipboardManager) getSystemService(Context.CLIPBOARD_SERVICE);
            if (cm == null || !cm.hasPrimaryClip()) return "";
            ClipData clip = cm.getPrimaryClip();
            if (clip == null || clip.getItemCount() == 0) return "";
            CharSequence t = clip.getItemAt(0).coerceToText(this);
            return t == null ? "" : t.toString();
        } catch (Exception e) {
            return "";
        }
    }

    @Override
    public void showKeyboard(boolean show) {
        runOnUiThread(() -> {
            if (view != null) view.showKeyboard(show);
        });
    }

    @Override
    public void quit() {
        runOnUiThread(this::finish);
    }

    private String pendingUpdate; // waiting for "install unknown apps" permission

    @Override
    public void installUpdate(final String path) {
        runOnUiThread(() -> {
            try {
                if (Build.VERSION.SDK_INT >= 26 && !getPackageManager().canRequestPackageInstalls()) {
                    // Android asks once per app: send them to the switch
                    pendingUpdate = path;
                    Toast.makeText(this, "Allow this app to install updates, then come back.", Toast.LENGTH_LONG).show();
                    startActivity(new Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,
                            Uri.parse("package:" + getPackageName())));
                    return;
                }
                Uri uri = Uri.parse("content://" + getPackageName() + ".update/update.apk");
                Intent i = new Intent(Intent.ACTION_VIEW);
                i.setDataAndType(uri, "application/vnd.android.package-archive");
                i.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_ACTIVITY_NEW_TASK);
                startActivity(i);
            } catch (Exception e) {
                Log.e("house", "install update", e);
                Toast.makeText(this, "Couldn't open the installer: " + e.getMessage(), Toast.LENGTH_LONG).show();
            }
        });
    }

    @Override
    @SuppressWarnings("deprecation")
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode != PICK_IMAGE || resultCode != RESULT_OK || data == null || data.getData() == null) {
            return;
        }
        Uri uri = data.getData();
        new Thread(() -> {
            try {
                File f = new File(getCacheDir(), "picked_" + System.currentTimeMillis());
                try (InputStream in = getContentResolver().openInputStream(uri);
                     OutputStream out = new FileOutputStream(f)) {
                    if (in == null) return;
                    byte[] buf = new byte[64 * 1024];
                    int total = 0;
                    for (int n; (n = in.read(buf)) > 0; ) {
                        total += n;
                        if (total > 8 * 1024 * 1024) break; // way bigger than needed
                        out.write(buf, 0, n);
                    }
                }
                Mobile.imagePicked(f.getAbsolutePath());
            } catch (Exception e) {
                Log.w(TAG, "picture", e);
            }
        }).start();
    }
}
