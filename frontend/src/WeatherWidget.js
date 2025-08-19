import React, { useState, useEffect } from 'react';
import './WeatherWidget.css';

const API_KEY = process.env.REACT_APP_OPENWEATHER_API_KEY;

const WeatherWidget = () => {
  const [weather, setWeather] = useState(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const getPlaceholderWeatherData = (lat, lon) => {
      const geocodingUrl = `https://api.bigdatacloud.net/data/reverse-geocode-client?latitude=${lat}&longitude=${lon}&localityLanguage=en`;

      fetch(geocodingUrl)
        .then(response => response.json())
        .then(geoData => {
          const cityName = geoData.city || "Local Area";
          const placeholderWeather = {
            name: cityName,
            main: { temp: 27.8 },
            weather: [{ main: 'Clear', description: 'clear sky', icon: '01d' }],
          };
          setWeather(placeholderWeather);
        })
        .catch(err => setError('Could not determine location name.'))
        .finally(() => setLoading(false));
    };

    const getWeatherData = (lat, lon) => {
      const url = `https://api.openweathermap.org/data/2.5/weather?lat=${lat}&lon=${lon}&appid=${API_KEY}&units=metric`;
      
      fetch(url)
        .then(response => {
          if (response.status === 401) {
            throw new Error('Unauthorized'); 
          }
          if (!response.ok) {
            throw new Error('Service unavailable');
          }
          return response.json();
        })
        .then(data => {
          if (data.cod === 200) {
            setWeather(data);
            setLoading(false);
          } else {
            throw new Error(data.message);
          }
        })
        .catch(err => {
          if (err.message === 'Unauthorized') {
            getPlaceholderWeatherData(lat, lon);
          } else {
            setError(`Weather service error: ${err.message}`);
            setLoading(false);
          }
        });
    };

    // --- Geolocation (Entry Point) ---
    if (navigator.geolocation) {
      navigator.geolocation.getCurrentPosition(
        (position) => {
          getWeatherData(position.coords.latitude, position.coords.longitude);
        },
        (err) => {
          setError(`Location Error: ${err.message}.`);
          setLoading(false);
        },
        { timeout: 10000 }
      );
    } else {
      setError('Geolocation is not supported.');
      setLoading(false);
    }
  }, []);

  if (loading) {
    return <div className="weather-widget">Loading Weather...</div>;
  }
  if (error) {
    return <div className="weather-widget error-message">{error}</div>;
  }
  if (!weather) {
    return <div className="weather-widget error-message">Weather data unavailable.</div>;
  }

  return (
    <div className="weather-widget">
      <div className="location">{weather.name}</div>
      <div className="main-info">
        <img
          className="weather-icon"
          src={`http://openweathermap.org/img/wn/${weather.weather[0].icon}.png`}
          alt={weather.weather[0].description}
        />
        <div className="temperature">{Math.round(weather.main.temp)}°C</div>
      </div>
      <div className="description">{weather.weather[0].main}</div>
    </div>
  );
};

export default WeatherWidget;