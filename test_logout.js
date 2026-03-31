const email = 'logout' + Date.now() + '@zydus.com';

(async () => {
  try {
    // Register
    const regRes = await fetch('http://localhost:8080/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: 'Logout Test', email, password: 'test123', role: 'admin' })
    });
    const regData = await regRes.json();
    console.log('✓ Register:', regRes.status);
    
    // Login with plain password
    const loginRes = await fetch('http://localhost:8080/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password: 'test123' })
    });
    const loginData = await loginRes.json();
    console.log('✓ Login:', loginRes.status);
    
    if (!loginData.token) {
      console.log('ERROR: No token received');
      return;
    }
    
    const token = loginData.token;
    
    // Logout API call
    const logoutRes = await fetch('http://localhost:8080/logout', {
      method: 'POST',
      headers: { 'Authorization': 'Bearer ' + token }
    });
    const logoutData = await logoutRes.json();
    console.log('✓ Logout API called:', logoutRes.status);
    console.log('  Response:', logoutData.message);
  } catch (err) {
    console.error('ERROR:', err.message);
  }
})();
