/*
 * Copyright 2020 The Chromium OS Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */


package org.chromium.arc.testapp.gamepad;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;
import android.app.Activity;
import android.content.Context;
import android.hardware.input.InputManager;
import android.os.Bundle;
import android.view.InputDevice;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.widget.TextView;
import java.util.List;

public class MainActivity extends Activity implements InputManager.InputDeviceListener {
    private InputManager mInputManager;
    private TextView mDeviceStatus;
    private TextView mKeyEvents;
    private TextView mMotionEvent;

    private JSONArray mKeyEventsArray = new JSONArray();

    private List<InputDevice.MotionRange> mMotionRanges;

    private final String KEY_ACTION = "action";
    private final String KEY_AXES = "axes";
    private final String KEY_DEVICE_ID = "device_id";
    private final String KEY_FLAT = "flat";
    private final String KEY_FUZZ = "fuzz";
    private final String KEY_KEY_CODE = "key_code";
    private final String KEY_MAX = "max";
    private final String KEY_MIN = "min";
    private final String KEY_MOTION_RANGES = "motion_ranges";
    private final String KEY_NAME = "name";
    private final String KEY_PRODUCT_ID = "product_id";
    private final String KEY_RANGE = "range";
    private final String KEY_RESOLUTION = "resolution";
    private final String KEY_SOURCE = "source";
    private final String KEY_VENDOR_ID = "vendor_id";

    @Override
    public void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.main_activity);

        mDeviceStatus = findViewById(R.id.device_status);
        mKeyEvents = findViewById(R.id.key_events);
        mMotionEvent = findViewById(R.id.motion_event);

        mInputManager = (InputManager) getSystemService(Context.INPUT_SERVICE);
    }

    @Override
    public void onResume() {
        super.onResume();
        mInputManager.registerInputDeviceListener(this, null);
        updateDeviceStatus();
    }

    @Override
    public void onPause() {
        mInputManager.unregisterInputDeviceListener(this);
        super.onPause();
    }

    @Override
    public boolean dispatchKeyEvent(KeyEvent event) {
        final int source = event.getSource();
        if ((source & InputDevice.SOURCE_GAMEPAD) == 0
                && (source & InputDevice.SOURCE_JOYSTICK) == 0) {
            return super.dispatchKeyEvent(event);
        }
        try {
            JSONObject eventObj = new JSONObject();
            eventObj.put(KEY_ACTION, keyEventActionToString(event.getAction()));
            eventObj.put(KEY_KEY_CODE, KeyEvent.keyCodeToString(event.getKeyCode()));
            eventObj.put(KEY_DEVICE_ID, event.getDeviceId());
            mKeyEventsArray.put(eventObj);
            mKeyEvents.setText(mKeyEventsArray.toString());
        } catch (JSONException e) {
        }
        return true;
    }

    @Override
    public boolean dispatchGenericMotionEvent(MotionEvent event) {
        final int source = event.getSource();
        if ((source & InputDevice.SOURCE_GAMEPAD) == 0
                && (source & InputDevice.SOURCE_JOYSTICK) == 0) {
            return super.dispatchGenericMotionEvent(event);
        }
        // It is possible that the InputDevice is already gone when MotionEvent arrives.
        InputDevice device = InputDevice.getDevice(event.getDeviceId());
        if (device != null) {
            mMotionRanges = device.getMotionRanges();
        }
        if (mMotionRanges == null) {
            return true;
        }
        try {
            JSONObject eventObj = new JSONObject();
            eventObj.put(KEY_ACTION, MotionEvent.actionToString(event.getAction()));
            JSONObject axesObj = new JSONObject();
            for (InputDevice.MotionRange motionRange : mMotionRanges) {
                final int axis = motionRange.getAxis();
                axesObj.put(MotionEvent.axisToString(axis), event.getAxisValue(axis));
            }
            eventObj.put(KEY_AXES, axesObj);
            mMotionEvent.setText(eventObj.toString());
        } catch (JSONException e) {
        }
        return true;
    }

    @Override
    public void onInputDeviceAdded(int i) {
        updateDeviceStatus();
    }

    @Override
    public void onInputDeviceRemoved(int i) {
        updateDeviceStatus();
    }

    @Override
    public void onInputDeviceChanged(int i) {
        updateDeviceStatus();
    }

    private void updateDeviceStatus() {
        try {
            JSONArray result = new JSONArray();
            for (int deviceId : InputDevice.getDeviceIds()) {
                InputDevice device = InputDevice.getDevice(deviceId);
                final int source = device.getSources();
                if (deviceId == 0
                        || (source & InputDevice.SOURCE_GAMEPAD) == 0
                        || (source & InputDevice.SOURCE_JOYSTICK) == 0) {
                    continue;
                }
                JSONObject deviceObj = new JSONObject();
                deviceObj.put(KEY_DEVICE_ID, device.getId());
                deviceObj.put(KEY_NAME, device.getName());
                deviceObj.put(KEY_PRODUCT_ID, device.getProductId());
                deviceObj.put(KEY_VENDOR_ID, device.getVendorId());

                JSONObject rangesObj = new JSONObject();
                for (InputDevice.MotionRange motionRange : device.getMotionRanges()) {
                    JSONObject rangeObj = new JSONObject();
                    rangeObj.put(KEY_FLAT, motionRange.getFlat());
                    rangeObj.put(KEY_FUZZ, motionRange.getFuzz());
                    rangeObj.put(KEY_MAX, motionRange.getMax());
                    rangeObj.put(KEY_MIN, motionRange.getMin());
                    rangeObj.put(KEY_RANGE, motionRange.getRange());
                    rangeObj.put(KEY_RESOLUTION, motionRange.getResolution());
                    rangeObj.put(KEY_SOURCE, motionRange.getSource());
                    rangesObj.put(MotionEvent.axisToString(motionRange.getAxis()), rangeObj);
                }
                deviceObj.put(KEY_MOTION_RANGES, rangesObj);

                result.put(deviceObj);
            }
            mDeviceStatus.setText(result.toString());
        } catch (JSONException e) {
        }
    }

    private String keyEventActionToString(int action) {
        switch (action) {
            case KeyEvent.ACTION_DOWN:
                return "ACTION_DOWN";
            case KeyEvent.ACTION_UP:
                return "ACTION_UP";
            default:
                return "";
        }
    }
}
