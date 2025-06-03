/**
 * Copyright 2024 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import { audioContext } from "./utils.js";
import { createWorkletFromSrc, registeredWorklets } from "./audioworklet-registry.js";
import AudioRecordingWorklet from "./audio-recording-worklet.js";

// Define a simple EventEmitter class since EventEmitter3 is not available
class EventEmitter {
  constructor() {
    this.events = {};
  }

  on(event, listener) {
    if (!this.events[event]) {
      this.events[event] = [];
    }
    this.events[event].push(listener);
    return this;
  }

  emit(event, ...args) {
    if (!this.events[event]) {
      return false;
    }
    this.events[event].forEach(listener => listener(...args));
    return true;
  }

  removeListener(event, listener) {
    if (!this.events[event]) {
      return this;
    }
    this.events[event] = this.events[event].filter(l => l !== listener);
    return this;
  }
}

function arrayBufferToBase64(buffer) {
  var binary = "";
  var bytes = new Uint8Array(buffer);
  var len = bytes.byteLength;
  for (var i = 0; i < len; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return window.btoa(binary);
}

// Simplified and more robust AudioRecorder class
class AudioRecorder {
  constructor() {
    this.events = {};
    this.isRecording = false;
    this.audioContext = null;
    this.mediaStream = null;
    this.source = null;
    this.processor = null;
    this.sendInterval = 500; // Default interval in ms
    this.audioChunks = [];
    this.sendTimer = null;
  }

  on(event, listener) {
    if (!this.events[event]) {
      this.events[event] = [];
    }
    this.events[event].push(listener);
    return this;
  }

  emit(event, ...args) {
    if (this.events[event]) {
      this.events[event].forEach(listener => listener(...args));
    }
    return this;
  }

  async start() {
    if (this.isRecording) {
      this.emit('log', 'Already recording');
      return;
    }

    try {
      // Create audio context
      this.audioContext = new (window.AudioContext || window.webkitAudioContext)({
        sampleRate: 16000, // Use 16kHz for better speech recognition
        latencyHint: 'interactive'
      });

      // Get user media
      this.emit('log', 'Requesting microphone access...');
      this.mediaStream = await navigator.mediaDevices.getUserMedia({
        audio: {
          echoCancellation: true,
          noiseSuppression: true,
          autoGainControl: true
        }
      });

      // Create source
      this.source = this.audioContext.createMediaStreamSource(this.mediaStream);
      
      // Create script processor for audio processing
      this.processor = this.audioContext.createScriptProcessor(4096, 1, 1);
      
      this.processor.onaudioprocess = (e) => {
        if (!this.isRecording) return;
        
        // Get audio data
        const audioData = e.inputBuffer.getChannelData(0);
        this.audioChunks.push(new Float32Array(audioData));
      };
      
      // Connect nodes
      this.source.connect(this.processor);
      this.processor.connect(this.audioContext.destination);
      
      // Start sending audio chunks
      this.isRecording = true;
      this.startSendingAudioChunks();
      
      this.emit('started');
      this.emit('log', 'Recording started');
    } catch (error) {
      this.emit('error', `Failed to start recording: ${error.message}`);
      throw error;
    }
  }

  stop() {
    if (!this.isRecording) {
      this.emit('log', 'Not recording');
      return;
    }
    
    this.isRecording = false;
    
    // Stop sending audio chunks
    if (this.sendTimer) {
      clearInterval(this.sendTimer);
      this.sendTimer = null;
    }
    
    // Disconnect and clean up
    if (this.processor) {
      this.processor.disconnect();
      this.processor = null;
    }
    
    if (this.source) {
      this.source.disconnect();
      this.source = null;
    }
    
    if (this.mediaStream) {
      this.mediaStream.getTracks().forEach(track => track.stop());
      this.mediaStream = null;
    }
    
    this.audioChunks = [];
    
    this.emit('stopped');
    this.emit('log', 'Recording stopped');
  }

  startSendingAudioChunks() {
    // Clear any existing timer
    if (this.sendTimer) {
      clearInterval(this.sendTimer);
    }
    
    this.sendTimer = setInterval(() => {
      if (!this.isRecording || this.audioChunks.length === 0) return;
      
      // Concatenate all chunks into a single Float32Array
      const totalLength = this.audioChunks.reduce((acc, chunk) => acc + chunk.length, 0);
      const concatenated = new Float32Array(totalLength);
      
      let offset = 0;
      for (const chunk of this.audioChunks) {
        concatenated.set(chunk, offset);
        offset += chunk.length;
      }
      
      // Convert to 16-bit PCM
      const pcm = new Int16Array(concatenated.length);
      for (let i = 0; i < concatenated.length; i++) {
        // Convert float to int16
        const s = Math.max(-1, Math.min(1, concatenated[i]));
        pcm[i] = s < 0 ? s * 0x8000 : s * 0x7FFF;
      }
      
      // Convert to base64
      const buffer = new ArrayBuffer(pcm.length * 2);
      const view = new DataView(buffer);
      pcm.forEach((value, index) => view.setInt16(index * 2, value, true));
      
      const blob = new Blob([buffer], { type: 'audio/webm' });
      const reader = new FileReader();
      
      reader.onloadend = () => {
        const base64data = reader.result.split(',')[1];
        this.emit('data', base64data);
      };
      
      reader.readAsDataURL(blob);
      
      // Clear chunks after sending
      this.audioChunks = [];
    }, this.sendInterval);
  }

  setSendInterval(interval) {
    this.sendInterval = interval;
    if (this.isRecording) {
      // Restart the timer with the new interval
      this.startSendingAudioChunks();
    }
  }
}

// Export the class
export { AudioRecorder };