import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<div>Login</div>} />
        <Route path="/setup" element={<div>Setup</div>} />
        <Route path="/live" element={<div>Live View</div>} />
        <Route path="/cameras" element={<div>Camera Management</div>} />
        <Route path="/recordings" element={<div>Recordings</div>} />
        <Route path="/settings" element={<div>Settings</div>} />
        <Route path="/users" element={<div>User Management</div>} />
        <Route path="/" element={<Navigate to="/live" replace />} />
      </Routes>
    </BrowserRouter>
  )
}

export default App
