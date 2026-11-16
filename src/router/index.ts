import { createRouter, createWebHistory } from 'vue-router'
import Login from '@/views/Login.vue'
import Overview from '@/views/Overview.vue'
import Containers from '@/views/Containers.vue'
import ContainerExec from '@/views/ContainerExec.vue'
import Images from '@/views/Images.vue'
import Networks from '@/views/Networks.vue'
import Volumes from '@/views/Volumes.vue'
import Projects from '@/views/Projects.vue'
import ProjectNew from '@/views/ProjectNew.vue'
import ProjectDetail from '@/views/ProjectDetail.vue'
import ContainerDetail from '@/views/ContainerDetail.vue'
import ContainerLogs from '@/views/ContainerLogs.vue'
import Logs from '@/views/Logs.vue'
import Settings from '@/views/Settings.vue'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
    history: createWebHistory(),
    routes: [
        { path: '/login', component: Login },
        { path: '/overview', component: Overview },
        { path: '/containers', component: Containers },
        { path: '/containers/:id', component: ContainerDetail },
        { path: '/containers/:id/logs', component: ContainerLogs },
        { path: '/containers/:id/exec', component: ContainerExec },
        { path: '/images', component: Images },
        { path: '/networks', component: Networks },
        { path: '/volumes', component: Volumes },
        { path: '/projects', component: Projects },
        { path: '/projects/new', component: ProjectNew },
        { path: '/projects/:name', component: ProjectDetail },
        { path: '/logs', component: Logs },
        { path: '/settings', component: Settings },
        { path: '/', redirect: '/overview' }
    ]
})

router.beforeEach(async (to, _from, next) => {
    const publicPages = ['/login'];
    const authRequired = !publicPages.includes(to.path);
    const authStore = useAuthStore();

    if (authRequired && !authStore.isAuthenticated) {
        // Try to verifying session
        const isValid = await authStore.checkAuth();
        if (!isValid) {
            return next('/login');
        }
    }

    if (to.path === '/login' && authStore.isAuthenticated) {
        return next('/');
    }

    next();
});

export default router
