import { createApp } from "vue";
import { createPinia } from "pinia";
import App from "./AdminApp.vue";
import "../shared/styles.css";
import "./admin.css";
createApp(App).use(createPinia()).mount("#app");
