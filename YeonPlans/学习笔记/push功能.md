**用代码控制页面进行跳转**
即：改变浏览器当前的网址路径，并渲染对应的页面

push会切换界面，然后切换依据是查询index.tx里面的东西

比如/online/home，
然后就找到name: 'OnlineHome',
在调取component: () => import('@/views/online/OnlineHome.vue'),
最终到达组件
<template>
<div>
<h1>🌐 在线首页</h1>
</div>
</template>
