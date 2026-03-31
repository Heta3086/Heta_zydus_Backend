(async () => {
  try {
    const loginRes = await fetch('http://localhost:8080/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: 'admin@zydus.com', password: 'admin123' })
    });
    const loginData = await loginRes.json();
    
    if (!loginData.token) {
      console.log('Login failed');
      return;
    }
    
    const token = loginData.token;
    
    const floorsRes = await fetch('http://localhost:8080/floor-numbers', {
      method: 'GET',
      headers: { 'Authorization': 'Bearer ' + token }
    });
    const floorsData = await floorsRes.json();
    console.log('✓ Floor Numbers API working');
    console.log('Available floors:', floorsData.floor_numbers);
  } catch (err) {
    console.error('Error:', err.message);
  }
})();
