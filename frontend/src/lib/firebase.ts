import { initializeApp } from "firebase/app";
import { getAuth } from "firebase/auth";

// Read from import.meta.env rather than hardcoding the console snippet, so
// Phase 9 switches to the prod project by changing an env file, not code.
// getAnalytics is deliberately absent: analytics is out of scope, adds a
// cookie-consent question, and breaks in test environments.
const firebaseConfig = {
  apiKey: import.meta.env.VITE_FIREBASE_API_KEY,
  authDomain: import.meta.env.VITE_FIREBASE_AUTH_DOMAIN,
  projectId: import.meta.env.VITE_FIREBASE_PROJECT_ID,
  appId: import.meta.env.VITE_FIREBASE_APP_ID,
};

export const app = initializeApp(firebaseConfig);
export const auth = getAuth(app);
