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

/**
 * Creates an AudioContext with the specified options
 * @param {Object} options - Options for creating the AudioContext
 * @param {number} options.sampleRate - The sample rate to use
 * @returns {Promise<AudioContext>} - A promise that resolves to an AudioContext
 */
export async function audioContext(options = {}) {
  const ctx = new (window.AudioContext || window.webkitAudioContext)({
    sampleRate: options.sampleRate || 44100,
    latencyHint: 'interactive'
  });
  
  // Some browsers require user interaction before starting the audio context
  if (ctx.state === 'suspended') {
    try {
      await ctx.resume();
    } catch (e) {
      console.warn('Could not resume audio context automatically:', e);
    }
  }
  
  return ctx;
}

/**
 * Utility function to safely play audio
 * @param {string} audioSrc - The audio source URL or data URI
 * @returns {Promise<HTMLAudioElement>} - A promise that resolves to the audio element
 */
export function playAudio(audioSrc) {
  return new Promise((resolve, reject) => {
    const audio = new Audio(audioSrc);
    
    audio.oncanplaythrough = () => {
      audio.play()
        .then(() => resolve(audio))
        .catch(err => reject(err));
    };
    
    audio.onerror = (err) => {
      reject(new Error(`Failed to load audio: ${err}`));
    };
  });
}

/**
 * Convert a base64 string to an ArrayBuffer
 * @param {string} base64 The base64 string to convert
 * @returns {ArrayBuffer} The converted ArrayBuffer
 */
export function base64ToArrayBuffer(base64) {
  const binaryString = atob(base64);
  const bytes = new Uint8Array(binaryString.length);
  for (let i = 0; i < binaryString.length; i++) {
    bytes[i] = binaryString.charCodeAt(i);
  }
  return bytes.buffer;
}