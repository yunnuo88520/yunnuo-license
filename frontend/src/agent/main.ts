import { createApp } from "vue";
import { createPinia } from "pinia";
import App from "./AgentApp.vue";
import "../shared/styles.css";
createApp(App).use(createPinia()).mount("#app");
