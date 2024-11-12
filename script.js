async function submitPricing() {
    const rows = document.querySelectorAll('#pricing-table-body tr');

    for (const row of rows) {
        const id = row.getAttribute('data-id');
        const sedanPriceInput = row.querySelector('.sedan-price');
        const suvPriceInput = row.querySelector('.suv-price');
        const startDateInput = row.querySelector('.start-date');
        
        const sedanPrice = parseFloat(sedanPriceInput.value) || 0;
        const suvPrice = parseFloat(suvPriceInput.value) || 0;

        // Correct calculations based on your definition
        const price = (sedanPrice / 1.05).toFixed(2); // Round to two decimals
        const pricingValue = ((suvPrice / 1.05) - price).toFixed(2); // Round to two decimals

        const startDate = new Date(startDateInput.value);
        const endDate = new Date(startDate);
        endDate.setDate(endDate.getDate() + 4); // Add 4 days

        const formattedStartDate = `${startDate.getFullYear()}-${(startDate.getMonth() + 1).toString().padStart(2, '0')}-${startDate.getDate().toString().padStart(2, '0')}T20:00:00.000Z`;
        const formattedEndDate = `${endDate.getFullYear()}-${(endDate.getMonth() + 1).toString().padStart(2, '0')}-${endDate.getDate().toString().padStart(2, '0')}T20:00:00.000Z`;

        // Call the first API
        const pricingId = await callFirstApi(id, price, pricingValue, formattedStartDate, formattedEndDate);
        
        // Call the second API with the returned pricing ID
        if (pricingId) {
            await callSecondApi(pricingId);
        }
    }
}

function calculatePrices() {
    const rows = document.querySelectorAll('#pricing-table-body tr');

    rows.forEach(row => {
        const sedanPriceInput = row.querySelector('.sedan-price');
        const suvPriceInput = row.querySelector('.suv-price');
        const calculatedPriceCell = row.querySelector('.calculated-price');
        const pricingValueCell = row.querySelector('.pricing-value');

        const sedanPrice = parseFloat(sedanPriceInput.value) || 0;
        const suvPrice = parseFloat(suvPriceInput.value) || 0;

        // Correct calculations based on your definition
        // const price = (sedanPrice / 1.05);
        // const pricingValue = ((suvPrice / 1.05) - (sedanPrice / 1.05));


        const price = (sedanPrice / 1.05).toFixed(2); // Round to two decimals
        const pricingValue = ((suvPrice / 1.05) - (sedanPrice / 1.05)).toFixed(2);


        calculatedPriceCell.textContent = price; // Display the rounded price
        pricingValueCell.textContent = pricingValue; // Display the rounded pricing value
    });
}

function updateEndDate(startDateInput) {
    const row = startDateInput.closest('tr');
    const endDateCell = row.querySelector('.end-date');
    
    const startDateValue = startDateInput.value;
    if (startDateValue) {
        const startDate = new Date(startDateValue);
        const endDate = new Date(startDate);
        endDate.setDate(endDate.getDate() + 4); // Add 4 days

        // Format the end date
        const formattedEndDate = `${endDate.getFullYear()}-${(endDate.getMonth() + 1).toString().padStart(2, '0')}-${endDate.getDate().toString().padStart(2, '0')}T20:00:00.000Z`;
        endDateCell.textContent = formattedEndDate;
    } else {
        endDateCell.textContent = '';
    }
}


function updateEndDateforWeekday(startDateInput) {
    const row = startDateInput.closest('tr');
    const endDateCell = row.querySelector('.end-date');
    
    const startDateValue = startDateInput.value;
    if (startDateValue) {
        const startDate = new Date(startDateValue);
        const endDate = new Date(startDate);
        endDate.setDate(endDate.getDate() + 3); // Add 4 days

        // Format the end date
        const formattedEndDate = `${endDate.getFullYear()}-${(endDate.getMonth() + 1).toString().padStart(2, '0')}-${endDate.getDate().toString().padStart(2, '0')}T20:00:00.000Z`;
        endDateCell.textContent = formattedEndDate;
    } else {
        endDateCell.textContent = '';
    }
}

// API Calls
async function callFirstApi(id, price, pricingValue, startDate, endDate) {
    const url = 'https://pricing.cafu.app/api/v1/pricing';
    const payload = {
        owner_type: "item-variations",
        owner_id: id,
        type: "per-unit",
        price: {
            amount: parseFloat(price), // Use the calculated price
            currency: "AED"
        },
        ranges: [],
        dynamic_pricing: [
            { pricing_type: "flat", pricing_value: parseFloat(pricingValue), object_type: "asset_subtype", object_value: "suv", price_currency: "AED" },
            { pricing_type: "flat", pricing_value: parseFloat(pricingValue), object_type: "asset_subtype", object_value: "van", price_currency: "AED" },
            { pricing_type: "flat", pricing_value: parseFloat(pricingValue), object_type: "asset_subtype", object_value: "pickup", price_currency: "AED" },
            { pricing_type: "flat", pricing_value: parseFloat(pricingValue), object_type: "asset_subtype", object_value: "minivan", price_currency: "AED" },
            { pricing_type: "flat", pricing_value: 0, object_type: "asset_subtype", object_value: "coupe", price_currency: "AED" },
            { pricing_type: "flat", pricing_value: 0, object_type: "asset_subtype", object_value: "hatchback", price_currency: "AED" },
            { pricing_type: "flat", pricing_value: 0, object_type: "asset_subtype", object_value: "liftback", price_currency: "AED" },
            { pricing_type: "flat", pricing_value: 0, object_type: "asset_subtype", object_value: "sedan", price_currency: "AED" }
        ],
        starts_at: startDate,
        ends_at: endDate,
        tax_exempt: false,
        active: true
    };

    const options = {
        method: 'POST',
        headers: {
            'Accept-Language': 'en-US',
            'Content-Type': 'application/json',
            'Accept': 'application/json'
        },
        body: JSON.stringify(payload)
    };

    try {
        const response = await fetch(url, options);
        const jsonResponse = await response.json();
        console.log('Pricing :', jsonResponse.pricing.id);
    
        const pricingId = jsonResponse.pricing ? jsonResponse.pricing.id : null;
        // var finalurl = `http://pricing.cafu.app/api/v1/pricing/${pricingId}/fees`;
// document.getElementById('message').innerHTML+= " Update the Pricing API so that the fees can be shown on the app otherwise use will be able to checkout without the service fees";
        document.getElementById('message').innerHTML+= 'Pricing ID: ' + pricingId + '<br>';
        return pricingId;
    } catch (error) {
        console.error('Error calling first API:', error);
        return null;
    }
}

async function callSecondApi(pricingId) {
    const url = `http://pricing.cafu.app/api/v1/pricing/${pricingId}/fees`;
    
    const payload = {
        fees_ids: [34]
    };

    const options = {
        method: 'PUT',
        headers: {
            'Accept': 'application/json',
            'Accept-Language': 'en-US',
            'Content-Type': 'application/json'
        },
        body: JSON.stringify(payload)
    };

    try {
        await fetch(url, options);
    } catch (error) {
        console.error('Error calling second API:', error);
    }
}