// demo/api/util.mjs
// mock 接口的响应构造器。约定：
//   json(body, status)  → JSON 响应
//   file(path, type)    → 读本地文件原样返回（封面/内页图）
//   raw(buffer, type)   → 直接返回字节
//   notFound(msg)       → 404 JSON

export const json = (body, status = 200) => ({ status, body })
export const file = (path, type) => ({ file: path, type })
export const raw = (body, type) => ({ raw: body, type })
export const notFound = (message = '演示数据未覆盖该接口') => json({ error: message }, 404)
